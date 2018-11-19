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
	MongoDBRolePhaseSuccess api.MongoDBRolePhase = "Success"
	finalizerInterval                            = 5 * time.Second
	finalizerTimeout                             = 30 * time.Minute
)

func (c *Controller) initMongoDBRoleWatcher() {
	c.mgRoleInformer = c.dbInformerFactory.Authorization().V1alpha1().MongoDBRoles().Informer()
	c.mgRoleQueue = queue.New(api.ResourceKindMongoDBRole, c.MaxNumRequeues, c.NumThreads, c.runMongoDBRoleInjector)
	c.mgRoleInformer.AddEventHandler(queue.NewObservableHandler(c.mgRoleQueue.GetQueue(), apis.EnableStatusSubresource))
	c.mgRoleLister = c.dbInformerFactory.Authorization().V1alpha1().MongoDBRoles().Lister()
}

func (c *Controller) runMongoDBRoleInjector(key string) error {
	obj, exist, err := c.mgRoleInformer.GetIndexer().GetByKey(key)
	if err != nil {
		glog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exist {
		glog.Warningf("MongoDBRole %s does not exist anymore", key)

	} else {
		mRole := obj.(*api.MongoDBRole).DeepCopy()

		glog.Infof("Sync/Add/Update for MongoDBRole %s/%s", mRole.Namespace, mRole.Name)

		if mRole.DeletionTimestamp != nil {
			if core_util.HasFinalizer(mRole.ObjectMeta, apis.Finalizer) {
				go c.runMongoDBRoleFinalizer(mRole, finalizerTimeout, finalizerInterval)
			}
		} else {
			if !core_util.HasFinalizer(mRole.ObjectMeta, apis.Finalizer) {
				// Add finalizer
				_, _, err := patchutil.PatchMongoDBRole(c.dbClient.AuthorizationV1alpha1(), mRole, func(role *api.MongoDBRole) *api.MongoDBRole {
					role.ObjectMeta = core_util.AddFinalizer(role.ObjectMeta, apis.Finalizer)
					return role
				})
				if err != nil {
					return errors.Wrapf(err, "failed to set MongoDBRole finalizer for %s/%s", mRole.Namespace, mRole.Name)
				}
			}

			dbRClient, err := database.NewDatabaseRoleForMongodb(c.kubeClient, c.catalogClient.AppcatalogV1alpha1(), mRole)
			if err != nil {
				return err
			}

			err = c.reconcileMongoDBRole(dbRClient, mRole)
			if err != nil {
				return errors.Wrapf(err, "for MongoDBRole %s/%s:", mRole.Namespace, mRole.Name)
			}
		}
	}
	return nil
}

// Will do:
//	For vault:
//	  - enable the database secrets engine if it is not already enabled
//	  - configure Vault with the proper Mongodb plugin and connection information
// 	  - configure a role that maps a name in Vault to an SQL statement to execute to create the database credential.
//    - sync role
//	  - revoke previous lease of all the respective mongodbRoleBinding and reissue a new lease
func (c *Controller) reconcileMongoDBRole(dbRClient database.DatabaseRoleInterface, mgRole *api.MongoDBRole) error {
	status := mgRole.Status
	// enable the database secrets engine if it is not already enabled
	err := dbRClient.EnableDatabase()
	if err != nil {
		status.Conditions = []api.MongoDBRoleCondition{
			{
				Type:    "Available",
				Status:  corev1.ConditionFalse,
				Reason:  "FailedToEnableDatabase",
				Message: err.Error(),
			},
		}

		err2 := c.updatedMongoDBRoleStatus(&status, mgRole)
		if err2 != nil {
			return errors.Wrap(err2, "failed to update status")
		}
		return errors.Wrap(err, "failed to enable database secret engine")
	}

	// create database config for Mongodb
	err = dbRClient.CreateConfig()
	if err != nil {
		status.Conditions = []api.MongoDBRoleCondition{
			{
				Type:    "Available",
				Status:  corev1.ConditionFalse,
				Reason:  "FailedToCreateDatabaseConfig",
				Message: err.Error(),
			},
		}

		err2 := c.updatedMongoDBRoleStatus(&status, mgRole)
		if err2 != nil {
			return errors.Wrap(err2, "failed to update status")
		}
		return errors.Wrap(err, "failed to create database connection config")
	}

	// create role
	err = dbRClient.CreateRole()
	if err != nil {
		status.Conditions = []api.MongoDBRoleCondition{
			{
				Type:    "Available",
				Status:  corev1.ConditionFalse,
				Reason:  "FailedToCreateRole",
				Message: err.Error(),
			},
		}

		err2 := c.updatedMongoDBRoleStatus(&status, mgRole)
		if err2 != nil {
			return errors.Wrap(err2, "failed to update status")
		}
		return errors.Wrap(err, "failed to create role")
	}

	status.Conditions = []api.MongoDBRoleCondition{}
	status.Phase = MongoDBRolePhaseSuccess
	status.ObservedGeneration = types.NewIntHash(mgRole.Generation, meta_util.GenerationHash(mgRole))

	err = c.updatedMongoDBRoleStatus(&status, mgRole)
	if err != nil {
		return errors.Wrapf(err, "failed to update MongoDBRole status")
	}
	return nil
}

func (c *Controller) updatedMongoDBRoleStatus(status *api.MongoDBRoleStatus, mRole *api.MongoDBRole) error {
	_, err := patchutil.UpdateMongoDBRoleStatus(c.dbClient.AuthorizationV1alpha1(), mRole, func(s *api.MongoDBRoleStatus) *api.MongoDBRoleStatus {
		s = status
		return s
	})
	if err != nil {
		return err
	}
	return nil
}

func (c *Controller) runMongoDBRoleFinalizer(mRole *api.MongoDBRole, timeout time.Duration, interval time.Duration) {
	id := getMongoDBRoleId(mRole)

	if _, ok := c.processingFinalizer[id]; ok {
		// already processing
		return
	}

	c.processingFinalizer[id] = true
	glog.Infof("MongoDBRole %s/%s finalizer: start processing\n", mRole.Namespace, mRole.Name)

	stopCh := time.After(timeout)
	finalizationDone := false
	attempt := 0

	for {
		m, err := c.dbClient.AuthorizationV1alpha1().MongoDBRoles(mRole.Namespace).Get(mRole.Name, metav1.GetOptions{})
		if kerr.IsNotFound(err) {
			delete(c.processingFinalizer, id)
			return
		} else if err != nil {
			glog.Errorf("MongoDBRole %s/%s finalizer: %v", mRole.Namespace, mRole.Name, err)
		}

		// to make sure p is not nil
		if m == nil {
			m = mRole
		}

		select {
		case <-stopCh:
			err := c.removeMongoDBRoleFinalizer(m)
			if err != nil {
				glog.Errorf("MongoDBRole %s/%s finalizer: %v", m.Namespace, m.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		default:
		}

		glog.Infof("MongoDBRole %s/%s finalizer: attempt %d\n", mRole.Namespace, mRole.Name, attempt)

		if !finalizationDone {
			d, err := database.NewDatabaseRoleForMongodb(c.kubeClient, c.catalogClient.AppcatalogV1alpha1(), m)
			if err != nil {
				glog.Errorf("MongoDBRole %s/%s finalizer: %v", m.Namespace, m.Name, err)
			} else {
				err = c.finalizeMongoDBRole(d, m)
				if err != nil {
					glog.Errorf("MongoDBRole %s/%s finalizer: %v", m.Namespace, m.Name, err)
				} else {
					finalizationDone = true
				}
			}

		}

		if finalizationDone {
			err := c.removeMongoDBRoleFinalizer(m)
			if err != nil {
				glog.Errorf("MongoDBRole %s/%s finalizer: removing finalizer %v", m.Namespace, m.Name, err)
			} else {
				delete(c.processingFinalizer, id)
				return
			}
		}

		select {
		case <-stopCh:
			err := c.removeMongoDBRoleFinalizer(m)
			if err != nil {
				glog.Errorf("MongoDBRole %s/%s finalizer: %v", m.Namespace, m.Name, err)
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
//	- revoke lease of all the corresponding mongodbRoleBinding
func (c *Controller) finalizeMongoDBRole(dbRClient database.DatabaseRoleInterface, mRole *api.MongoDBRole) error {
	err := dbRClient.DeleteRole(mRole.RoleName())
	if err != nil {
		return errors.Wrap(err, "failed to database role")
	}
	return nil
}

func (c *Controller) removeMongoDBRoleFinalizer(mRole *api.MongoDBRole) error {
	// remove finalizer
	_, _, err := patchutil.PatchMongoDBRole(c.dbClient.AuthorizationV1alpha1(), mRole, func(role *api.MongoDBRole) *api.MongoDBRole {
		role.ObjectMeta = core_util.RemoveFinalizer(role.ObjectMeta, apis.Finalizer)
		return role
	})
	if err != nil {
		return err
	}
	return nil
}

func getMongoDBRoleId(mRole *api.MongoDBRole) string {
	return fmt.Sprintf("%s/%s/%s", api.ResourceMongoDBRole, mRole.Namespace, mRole.Name)
}
