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
	"k8s.io/apimachinery/pkg/labels"
)

const (
	MySQLRolePhaseSuccess api.MySQLRolePhase = "Success"
)

func (c *Controller) initMySQLRoleWatcher() {
	c.myRoleInformer = c.dbInformerFactory.Authorization().V1alpha1().MySQLRoles().Informer()
	c.myRoleQueue = queue.New(api.ResourceKindMySQLRole, c.MaxNumRequeues, c.NumThreads, c.runMySQLRoleInjector)
	c.myRoleInformer.AddEventHandler(queue.NewObservableHandler(c.myRoleQueue.GetQueue(), apis.EnableStatusSubresource))
	c.myRoleLister = c.dbInformerFactory.Authorization().V1alpha1().MySQLRoles().Lister()
}

func (c *Controller) runMySQLRoleInjector(key string) error {
	obj, exist, err := c.myRoleInformer.GetIndexer().GetByKey(key)
	if err != nil {
		glog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exist {
		glog.Warningf("MySQLRole %s does not exist anymore", key)

	} else {
		mRole := obj.(*api.MySQLRole).DeepCopy()

		glog.Infof("Sync/Add/Update for MySQLRole %s/%s", mRole.Namespace, mRole.Name)

		if mRole.DeletionTimestamp != nil {
			if core_util.HasFinalizer(mRole.ObjectMeta, apis.Finalizer) {
				go c.runMySQLRoleFinalizer(mRole, 1*time.Minute, 10*time.Second)
			}

		} else {
			if !core_util.HasFinalizer(mRole.ObjectMeta, apis.Finalizer) {
				// Add finalizer
				_, _, err := patchutil.PatchMySQLRole(c.dbClient.AuthorizationV1alpha1(), mRole, func(role *api.MySQLRole) *api.MySQLRole {
					role.ObjectMeta = core_util.AddFinalizer(role.ObjectMeta, apis.Finalizer)
					return role
				})
				if err != nil {
					return errors.Wrapf(err, "failed to set MySQLRole finalizer for %s/%s", mRole.Namespace, mRole.Name)
				}
			}

			dbRClient, err := database.NewDatabaseRoleForMysql(c.kubeClient, mRole)
			if err != nil {
				return err
			}

			err = c.reconcileMySQLRole(dbRClient, mRole)
			if err != nil {
				return errors.Wrapf(err, "for MySQLRole %s/%s:", mRole.Namespace, mRole.Name)
			}
		}
	}
	return nil
}

// Will do:
//	For vault:
//	  - enable the database secrets engine if it is not already enabled
//	  - configure Vault with the proper mysql plugin and connection information
// 	  - configure a role that maps a name in Vault to an SQL statement to execute to create the database credential.
//    - sync role
//	  - revoke previous lease of all the respective mysqlRoleBinding and reissue a new lease
func (c *Controller) reconcileMySQLRole(dbRClient database.DatabaseRoleInterface, myRole *api.MySQLRole) error {
	status := myRole.Status
	// enable the database secrets engine if it is not already enabled
	err := dbRClient.EnableDatabase()
	if err != nil {
		status.Conditions = []api.MySQLRoleCondition{
			{
				Type:    "Available",
				Status:  corev1.ConditionFalse,
				Reason:  "FailedToEnableDatabase",
				Message: err.Error(),
			},
		}

		err2 := c.updatedMySQLRoleStatus(&status, myRole)
		if err2 != nil {
			return errors.Wrap(err2, "failed to update status")
		}
		return errors.Wrap(err, "failed to enable database secret engine")
	}

	// create database config for mysql
	err = dbRClient.CreateConfig()
	if err != nil {
		status.Conditions = []api.MySQLRoleCondition{
			{
				Type:    "Available",
				Status:  corev1.ConditionFalse,
				Reason:  "FailedToCreateDatabaseConfig",
				Message: err.Error(),
			},
		}

		err2 := c.updatedMySQLRoleStatus(&status, myRole)
		if err2 != nil {
			return errors.Wrap(err2, "failed to update status")
		}
		return errors.Wrapf(err, "failed to created database connection config %s", myRole.Spec.Database.Name)
	}

	// create role
	err = dbRClient.CreateRole()
	if err != nil {
		status.Conditions = []api.MySQLRoleCondition{
			{
				Type:    "Available",
				Status:  corev1.ConditionFalse,
				Reason:  "FailedToCreateRole",
				Message: err.Error(),
			},
		}

		err2 := c.updatedMySQLRoleStatus(&status, myRole)
		if err2 != nil {
			return errors.Wrap(err2, "failed to update status")
		}
		return errors.Wrap(err, "failed to create role")
	}

	status.Conditions = []api.MySQLRoleCondition{}
	status.Phase = MySQLRolePhaseSuccess
	status.ObservedGeneration = types.NewIntHash(myRole.Generation, meta_util.GenerationHash(myRole))

	err = c.updatedMySQLRoleStatus(&status, myRole)
	if err != nil {
		return errors.Wrap(err, "failed to update MySQLRole status")
	}

	mList, err := c.myRoleBindingLister.MySQLRoleBindings(myRole.Namespace).List(labels.Everything())
	for _, m := range mList {
		if m.Spec.RoleRef == myRole.Name {
			// revoke lease if have any lease
			if m.Status.Lease.ID != "" {
				err = c.RevokeLease(myRole.Spec.AuthManagerRef, m.Status.Lease.ID)
				if err != nil {
					return errors.Wrap(err, "failed to revoke lease")
				}

				status := m.Status
				status.Lease = api.LeaseData{}
				err = c.updateMySQLRoleBindingStatus(&status, m)
				if err != nil {
					return errors.WithStack(err)
				}
			}

			// enqueue mysqlRoleBinding to reissue database credentials lease
			queue.Enqueue(c.myRoleBindingQueue.GetQueue(), m)
		}
	}
	return nil
}

func (c *Controller) updatedMySQLRoleStatus(status *api.MySQLRoleStatus, mRole *api.MySQLRole) error {
	_, err := patchutil.UpdateMySQLRoleStatus(c.dbClient.AuthorizationV1alpha1(), mRole, func(s *api.MySQLRoleStatus) *api.MySQLRoleStatus {
		s = status
		return s
	})
	if err != nil {
		return err
	}
	return nil
}

func (c *Controller) runMySQLRoleFinalizer(mRole *api.MySQLRole, timeout time.Duration, interval time.Duration) {
	id := getMySQLRoleId(mRole)

	if _, ok := c.processingFinalizer[id]; ok {
		// already processing
		return
	}

	c.processingFinalizer[id] = true

	stopCh := time.After(timeout)
	finalizationDone := false

	for {
		m, err := c.dbClient.AuthorizationV1alpha1().MySQLRoles(mRole.Namespace).Get(mRole.Name, metav1.GetOptions{})
		if kerr.IsNotFound(err) {
			delete(c.processingFinalizer, id)
			return
		} else if err != nil {
			glog.Errorf("MySQLRole %s/%s finalizer: %v", mRole.Namespace, mRole.Name, err)
		}

		// to make sure p is not nil
		if m == nil {
			m = mRole
		}

		select {
		case <-stopCh:
			err := c.removeMySQLRoleFinalizer(m)
			if err != nil {
				glog.Errorf("MySQLRole %s/%s finalizer: %v", m.Namespace, m.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		default:
		}

		if !finalizationDone {
			d, err := database.NewDatabaseRoleForMysql(c.kubeClient, m)
			if err != nil {
				glog.Errorf("MySQLRole %s/%s finalizer: %v", m.Namespace, m.Name, err)
			} else {
				err = c.finalizeMySQLRole(d, m)
				if err != nil {
					glog.Errorf("MySQLRole %s/%s finalizer: %v", m.Namespace, m.Name, err)
				} else {
					finalizationDone = true
				}
			}

		}

		if finalizationDone {
			err := c.removeMySQLRoleFinalizer(m)
			if err != nil {
				glog.Errorf("MySQLRole %s/%s finalizer: %v", m.Namespace, m.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		}

		select {
		case <-stopCh:
			err := c.removeMySQLRoleFinalizer(m)
			if err != nil {
				glog.Errorf("MySQLRole %s/%s finalizer: %v", m.Namespace, m.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		case <-time.After(interval):
		}
	}
}

// Do:
//	- delete role in vault
//	- revoke lease of all the corresponding mysqlRoleBinding
func (c *Controller) finalizeMySQLRole(dbRClient database.DatabaseRoleInterface, mRole *api.MySQLRole) error {
	mRList, err := c.myRoleBindingLister.MySQLRoleBindings(mRole.Namespace).List(labels.Everything())
	if err != nil {
		return errors.Wrap(err, "failed to list mysqlRoleBinding")
	}

	for _, m := range mRList {
		if m.Spec.RoleRef == mRole.Name {
			if m.Status.Lease.ID != "" {
				err = c.RevokeLease(mRole.Spec.AuthManagerRef, m.Status.Lease.ID)
				if err != nil {
					return errors.Wrap(err, "failed to revoke lease")
				}

				status := m.Status
				status.Lease = api.LeaseData{}
				err = c.updateMySQLRoleBindingStatus(&status, m)
				if err != nil {
					return errors.WithStack(err)
				}
			}
		}
	}

	err = dbRClient.DeleteRole(mRole.Name)
	if err != nil {
		return errors.Wrap(err, "failed to database role")
	}
	return nil
}

func (c *Controller) removeMySQLRoleFinalizer(mRole *api.MySQLRole) error {
	// remove finalizer
	_, _, err := patchutil.PatchMySQLRole(c.dbClient.AuthorizationV1alpha1(), mRole, func(role *api.MySQLRole) *api.MySQLRole {
		role.ObjectMeta = core_util.RemoveFinalizer(role.ObjectMeta, apis.Finalizer)
		return role
	})
	if err != nil {
		return err
	}
	return nil
}

func getMySQLRoleId(mRole *api.MySQLRole) string {
	return fmt.Sprintf("%s/%s/%s", api.ResourceMySQLRole, mRole.Namespace, mRole.Name)
}
