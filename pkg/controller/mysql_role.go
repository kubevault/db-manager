package controller

import (
	"fmt"
	"time"

	kutilcorev1 "github.com/appscode/kutil/core/v1"
	"github.com/appscode/kutil/tools/queue"
	"github.com/golang/glog"
	api "github.com/kubedb/user-manager/apis/authorization/v1alpha1"
	patchutil "github.com/kubedb/user-manager/client/clientset/versioned/typed/authorization/v1alpha1/util"
	"github.com/kubedb/user-manager/pkg/vault/database"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	MysqlRoleFinalizer = "database.mysql.role"

	MysqlRolePhaseSuccess api.MysqlRolePhase = "Success"
)

func (c *UserManagerController) initMysqlRoleWatcher() {
	c.myRoleInformer = c.dbInformerFactory.Authorization().V1alpha1().MysqlRoles().Informer()
	c.myRoleQueue = queue.New(api.ResourceKindMysqlRole, c.MaxNumRequeues, c.NumThreads, c.runMysqlRoleInjector)
	c.myRoleInformer.AddEventHandler(queue.NewEventHandler(c.myRoleQueue.GetQueue(), func(old interface{}, new interface{}) bool {
		oldObj := old.(*api.MysqlRole)
		newObj := new.(*api.MysqlRole)
		return newObj.DeletionTimestamp != nil || !newObj.AlreadyObserved(oldObj)
	}))
	c.myRoleLister = c.dbInformerFactory.Authorization().V1alpha1().MysqlRoles().Lister()
}

func (c *UserManagerController) runMysqlRoleInjector(key string) error {
	obj, exist, err := c.myRoleInformer.GetIndexer().GetByKey(key)
	if err != nil {
		glog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exist {
		glog.Warningf("MysqlRole %s does not exist anymore\n", key)

	} else {
		mRole := obj.(*api.MysqlRole).DeepCopy()

		glog.Infof("Sync/Add/Update for MysqlRole %s/%s\n", mRole.Namespace, mRole.Name)

		if mRole.DeletionTimestamp != nil {
			if kutilcorev1.HasFinalizer(mRole.ObjectMeta, MysqlRoleFinalizer) {
				go c.runMysqlRoleFinalizer(mRole, 1*time.Minute, 10*time.Second)
			}

		} else {
			if !kutilcorev1.HasFinalizer(mRole.ObjectMeta, MysqlRoleFinalizer) {
				// Add finalizer
				_, _, err := patchutil.PatchMysqlRole(c.dbClient.AuthorizationV1alpha1(), mRole, func(role *api.MysqlRole) *api.MysqlRole {
					role.ObjectMeta = kutilcorev1.AddFinalizer(role.ObjectMeta, MysqlRoleFinalizer)
					return role
				})
				if err != nil {
					return errors.Wrapf(err, "failed to set MysqlRole finalizer for (%s/%s)", mRole.Namespace, mRole.Name)
				}
			}

			dbRClient, err := database.NewDatabaseRoleForMysql(c.kubeClient, mRole)
			if err != nil {
				return err
			}

			err = c.reconcileMysqlRole(dbRClient, mRole)
			if err != nil {
				return errors.Wrapf(err, "for MysqlRole(%s/%s):", mRole.Namespace, mRole.Name)
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
func (c *UserManagerController) reconcileMysqlRole(dbRClient database.DatabaseRoleInterface, mRole *api.MysqlRole) error {
	status := mRole.Status
	// enable the database secrets engine if it is not already enabled
	err := dbRClient.EnableDatabase()
	if err != nil {
		status.Conditions = []api.MysqlRoleCondition{
			{
				Type:    "Available",
				Status:  corev1.ConditionFalse,
				Reason:  "FailedToEnableDatabase",
				Message: err.Error(),
			},
		}

		err2 := c.updatedMysqlRoleStatus(&status, mRole)
		if err2 != nil {
			return errors.Wrap(err2, "failed to update status")
		}
		return errors.Wrap(err, "failed to enable database secret engine")
	}

	// create database config for mysql
	err = dbRClient.CreateConfig()
	if err != nil {
		status.Conditions = []api.MysqlRoleCondition{
			{
				Type:    "Available",
				Status:  corev1.ConditionFalse,
				Reason:  "FailedToCreateDatabaseConfig",
				Message: err.Error(),
			},
		}

		err2 := c.updatedMysqlRoleStatus(&status, mRole)
		if err2 != nil {
			return errors.Wrap(err2, "failed to update status")
		}
		return errors.Wrapf(err, "failed to created database connection config(%s)", mRole.Spec.Database.Name)
	}

	// create role
	err = dbRClient.CreateRole()
	if err != nil {
		status.Conditions = []api.MysqlRoleCondition{
			{
				Type:    "Available",
				Status:  corev1.ConditionFalse,
				Reason:  "FailedToCreateRole",
				Message: err.Error(),
			},
		}

		err2 := c.updatedMysqlRoleStatus(&status, mRole)
		if err2 != nil {
			return errors.Wrap(err2, "failed to update status")
		}
		return errors.Wrap(err, "failed to create role")
	}

	status.Conditions = []api.MysqlRoleCondition{}
	status.Phase = MysqlRolePhaseSuccess
	status.ObservedGeneration = mRole.Generation

	err = c.updatedMysqlRoleStatus(&status, mRole)
	if err != nil {
		return errors.Wrap(err, "failed to update MysqlRole status")
	}

	return nil
}

func (c *UserManagerController) updatedMysqlRoleStatus(status *api.MysqlRoleStatus, mRole *api.MysqlRole) error {
	_, err := patchutil.UpdateMysqlRoleStatus(c.dbClient.AuthorizationV1alpha1(), mRole, func(s *api.MysqlRoleStatus) *api.MysqlRoleStatus {
		s = status
		return s
	})
	if err != nil {
		return err
	}

	return nil
}

func (c *UserManagerController) runMysqlRoleFinalizer(mRole *api.MysqlRole, timeout time.Duration, interval time.Duration) {
	id := getMysqlRoleId(mRole)

	if _, ok := c.processingFinalizer[id]; ok {
		// already processing
		return
	}

	c.processingFinalizer[id] = true

	stopCh := time.After(timeout)
	finalizationDone := false

	for {
		m, err := c.dbClient.AuthorizationV1alpha1().MysqlRoles(mRole.Namespace).Get(mRole.Name, metav1.GetOptions{})
		if kerr.IsNotFound(err) {
			delete(c.processingFinalizer, id)
			return
		} else if err != nil {
			glog.Errorf("MysqlRole(%s/%s) finalizer: %v\n", mRole.Namespace, mRole.Name, err)
		}

		// to make sure p is not nil
		if m == nil {
			m = mRole
		}

		select {
		case <-stopCh:
			err := c.removeMysqlRoleFinalizer(m)
			if err != nil {
				glog.Errorf("MysqlRole(%s/%s) finalizer: %v\n", m.Namespace, m.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		default:
		}

		if !finalizationDone {
			d, err := database.NewDatabaseRoleForMysql(c.kubeClient, m)
			if err != nil {
				glog.Errorf("MysqlRole(%s/%s) finalizer: %v\n", m.Namespace, m.Name, err)
			} else {
				err = c.finalizeMysqlRole(d, m)
				if err != nil {
					glog.Errorf("MysqlRole(%s/%s) finalizer: %v\n", m.Namespace, m.Name, err)
				} else {
					finalizationDone = true
				}
			}

		}

		if finalizationDone {
			err := c.removeMysqlRoleFinalizer(m)
			if err != nil {
				glog.Errorf("MysqlRole(%s/%s) finalizer: %v\n", m.Namespace, m.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		}

		select {
		case <-stopCh:
			err := c.removeMysqlRoleFinalizer(m)
			if err != nil {
				glog.Errorf("MysqlRole(%s/%s) finalizer: %v\n", m.Namespace, m.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		case <-time.After(interval):
		}
	}
}

func (c *UserManagerController) finalizeMysqlRole(dbRClient database.DatabaseRoleInterface, mRole *api.MysqlRole) error {
	err := dbRClient.DeleteRole(mRole.Name)
	if err != nil {
		return errors.Wrap(err, "failed to database role")
	}

	return nil
}

func (c *UserManagerController) removeMysqlRoleFinalizer(mRole *api.MysqlRole) error {
	// remove finalizer
	_, _, err := patchutil.PatchMysqlRole(c.dbClient.AuthorizationV1alpha1(), mRole, func(role *api.MysqlRole) *api.MysqlRole {
		role.ObjectMeta = kutilcorev1.RemoveFinalizer(role.ObjectMeta, MysqlRoleFinalizer)
		return role
	})
	if err != nil {
		return err
	}

	return nil
}

func getMysqlRoleId(mRole *api.MysqlRole) string {
	return fmt.Sprintf("%s/%s/%s", api.ResourceMysqlRole, mRole.Namespace, mRole.Name)
}
