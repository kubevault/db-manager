package controller

import (
	"fmt"
	"time"

	"github.com/appscode/go/encoding/json/types"
	core_util "github.com/appscode/kutil/core/v1"
	meta_util "github.com/appscode/kutil/meta"
	"github.com/appscode/kutil/tools/queue"
	"github.com/golang/glog"
	"github.com/kubedb/apimachinery/apis"
	api "github.com/kubedb/apimachinery/apis/authorization/v1alpha1"
	patchutil "github.com/kubedb/apimachinery/client/clientset/versioned/typed/authorization/v1alpha1/util"
	"github.com/kubevault/db-manager/pkg/vault/database"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	PostgresRolePhaseSuccess api.PostgresRolePhase = "Success"
)

func (c *Controller) initPostgresRoleWatcher() {
	c.pgRoleInformer = c.dbInformerFactory.Authorization().V1alpha1().PostgresRoles().Informer()
	c.pgRoleQueue = queue.New(api.ResourceKindPostgresRole, c.MaxNumRequeues, c.NumThreads, c.runPostgresRoleInjector)
	c.pgRoleInformer.AddEventHandler(queue.NewObservableHandler(c.pgRoleQueue.GetQueue(), apis.EnableStatusSubresource))
	c.pgRoleLister = c.dbInformerFactory.Authorization().V1alpha1().PostgresRoles().Lister()
}

func (c *Controller) runPostgresRoleInjector(key string) error {
	obj, exist, err := c.pgRoleInformer.GetIndexer().GetByKey(key)
	if err != nil {
		glog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exist {
		glog.Warningf("PostgresRole %s does not exist anymore", key)

	} else {
		pgRole := obj.(*api.PostgresRole).DeepCopy()

		glog.Infof("Sync/Add/Update for PostgresRole %s/%s", pgRole.Namespace, pgRole.Name)

		if pgRole.DeletionTimestamp != nil {
			if core_util.HasFinalizer(pgRole.ObjectMeta, apis.Finalizer) {
				go c.runPostgresRoleFinalizer(pgRole, finalizerTimeout, finalizerInterval)
			}

		} else {
			if !core_util.HasFinalizer(pgRole.ObjectMeta, apis.Finalizer) {
				// Add finalizer
				_, _, err := patchutil.PatchPostgresRole(c.dbClient.AuthorizationV1alpha1(), pgRole, func(role *api.PostgresRole) *api.PostgresRole {
					role.ObjectMeta = core_util.AddFinalizer(role.ObjectMeta, apis.Finalizer)
					return role
				})
				if err != nil {
					return errors.Wrapf(err, "failed to set postgresRole finalizer for %s/%s", pgRole.Namespace, pgRole.Name)
				}
			}

			dbRClient, err := database.NewDatabaseRoleForPostgres(c.kubeClient, c.catalogClient.AppcatalogV1alpha1(), pgRole)
			if err != nil {
				return err
			}

			err = c.reconcilePostgresRole(dbRClient, pgRole)
			if err != nil {
				return errors.Wrapf(err, "for PostgresRole %s/%s:", pgRole.Namespace, pgRole.Name)
			}
		}
	}
	return nil
}

// Will do:
//	For vault:
//	  - enable the database secrets engine if it is not already enabled
//	  - configure Vault with the proper postgres plugin and connection information
// 	  - configure a role that maps a name in Vault to an SQL statement to execute to create the database credential.
//    - sync role
//	  - revoke previous lease of all the respective postgresRoleBinding and reissue a new lease
func (c *Controller) reconcilePostgresRole(dbRClient database.DatabaseRoleInterface, pgRole *api.PostgresRole) error {
	status := pgRole.Status
	// enable the database secrets engine if it is not already enabled
	err := dbRClient.EnableDatabase()
	if err != nil {
		status.Conditions = []api.PostgresRoleCondition{
			{
				Type:    "Available",
				Status:  corev1.ConditionFalse,
				Reason:  "FailedToEnableDatabase",
				Message: err.Error(),
			},
		}

		err2 := c.updatePostgresRoleStatus(&status, pgRole)
		if err2 != nil {
			return errors.Wrap(err2, "failed to update status")
		}
		return errors.Wrap(err, "failed to enable database secret engine")
	}

	// create database config for postgres
	err = dbRClient.CreateConfig()
	if err != nil {
		status.Conditions = []api.PostgresRoleCondition{
			{
				Type:    "Available",
				Status:  corev1.ConditionFalse,
				Reason:  "FailedToCreateDatabaseConnectionConfig",
				Message: err.Error(),
			},
		}

		err2 := c.updatePostgresRoleStatus(&status, pgRole)
		if err2 != nil {
			return errors.Wrap(err2, "failed to update status")
		}
		return errors.Wrap(err, "failed to created database connection config")
	}

	// create role
	err = dbRClient.CreateRole()
	if err != nil {
		status.Conditions = []api.PostgresRoleCondition{
			{
				Type:    "Available",
				Status:  corev1.ConditionFalse,
				Reason:  "FailedToCreateDatabaseRole",
				Message: err.Error(),
			},
		}

		err2 := c.updatePostgresRoleStatus(&status, pgRole)
		if err2 != nil {
			return errors.Wrap(err2, "for postgresRole %s/%s: failed to update status")
		}
		return errors.Wrap(err, "for postgresRole %s/%s: failed to create role")
	}

	status.ObservedGeneration = types.NewIntHash(pgRole.Generation, meta_util.GenerationHash(pgRole))
	status.Conditions = []api.PostgresRoleCondition{}
	status.Phase = PostgresRolePhaseSuccess

	err = c.updatePostgresRoleStatus(&status, pgRole)
	if err != nil {
		return errors.Wrap(err, "failed to update postgresRole status")
	}
	return nil
}

func (c *Controller) updatePostgresRoleStatus(status *api.PostgresRoleStatus, pgRole *api.PostgresRole) error {
	_, err := patchutil.UpdatePostgresRoleStatus(c.dbClient.AuthorizationV1alpha1(), pgRole, func(s *api.PostgresRoleStatus) *api.PostgresRoleStatus {
		s = status
		return s
	}, apis.EnableStatusSubresource)
	if err != nil {
		return err
	}

	return nil
}

func (c *Controller) runPostgresRoleFinalizer(pgRole *api.PostgresRole, timeout time.Duration, interval time.Duration) {
	id := getPostgresRoleId(pgRole)

	if _, ok := c.processingFinalizer[id]; ok {
		// already processing
		return
	}

	c.processingFinalizer[id] = true
	glog.Infof("PostgresRole %s/%s finalizer: process starting...", pgRole.Namespace, pgRole.Name)

	stopCh := time.After(timeout)
	finalizationDone := false
	attempt := 0

	for {
		p, err := c.dbClient.AuthorizationV1alpha1().PostgresRoles(pgRole.Namespace).Get(pgRole.Name, metav1.GetOptions{})
		if kerr.IsNotFound(err) {
			delete(c.processingFinalizer, id)
			return
		} else if err != nil {
			glog.Errorf("PostgresRole %s/%s finalizer: %v", pgRole.Namespace, pgRole.Name, err)
		}

		// to make sure p is not nil
		if p == nil {
			p = pgRole
		}

		select {
		case <-stopCh:
			err := c.removePostgresRoleFinalizer(p)
			if err != nil {
				glog.Errorf("PostgresRole %s/%s finalizer: %v", p.Namespace, p.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		default:
		}

		glog.Infof("PostgresRole %s/%s finalizer: attempt %d", pgRole.Namespace, pgRole.Name, attempt)

		if !finalizationDone {
			d, err := database.NewDatabaseRoleForPostgres(c.kubeClient, c.catalogClient.AppcatalogV1alpha1(), p)
			if err != nil {
				glog.Errorf("PostgresRole %s/%s finalizer: %v", p.Namespace, p.Name, err)
			} else {
				err = c.finalizePostgresRole(d, p)
				if err != nil {
					glog.Errorf("PostgresRole %s/%s finalizer: %v", p.Namespace, p.Name, err)
				} else {
					finalizationDone = true
				}
			}
		}

		if finalizationDone {
			err := c.removePostgresRoleFinalizer(p)
			if err != nil {
				glog.Errorf("PostgresRole %s/%s finalizer: %v", p.Namespace, p.Name, err)
			} else {
				delete(c.processingFinalizer, id)
				return
			}
		}

		select {
		case <-stopCh:
			err := c.removePostgresRoleFinalizer(p)
			if err != nil {
				glog.Errorf("PostgresRole %s/%s finalizer: %v", p.Namespace, p.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		case <-time.After(interval):
		}
		attempt++
	}
}

// Do:
//	- delete role in vault
//	- revoke lease of all the corresponding postgresRoleBinding
func (c *Controller) finalizePostgresRole(dbRClient database.DatabaseRoleInterface, pgRole *api.PostgresRole) error {
	err := dbRClient.DeleteRole(pgRole.RoleName())
	if err != nil {
		return errors.Wrap(err, "failed to database role")
	}
	return nil
}

func (c *Controller) removePostgresRoleFinalizer(pgRole *api.PostgresRole) error {
	// remove finalizer
	_, _, err := patchutil.PatchPostgresRole(c.dbClient.AuthorizationV1alpha1(), pgRole, func(role *api.PostgresRole) *api.PostgresRole {
		role.ObjectMeta = core_util.RemoveFinalizer(role.ObjectMeta, apis.Finalizer)
		return role
	})
	if err != nil {
		return err
	}
	return nil
}

func getPostgresRoleId(pgRole *api.PostgresRole) string {
	return fmt.Sprintf("%s/%s/%s", api.ResourcePostgresRole, pgRole.Namespace, pgRole.Name)
}
