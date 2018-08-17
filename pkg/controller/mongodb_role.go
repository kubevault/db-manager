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
	MongodbRoleFinalizer = "database.mongodb.role"

	MongodbRolePhaseSuccess api.MongodbRolePhase = "Success"
)

func (c *UserManagerController) initMongodbRoleWatcher() {
	c.mgRoleInformer = c.dbInformerFactory.Authorization().V1alpha1().MongodbRoles().Informer()
	c.mgRoleQueue = queue.New(api.ResourceKindMongodbRole, c.MaxNumRequeues, c.NumThreads, c.runMongodbRoleInjector)

	c.mgRoleInformer.AddEventHandler(queue.NewEventHandler(c.mgRoleQueue.GetQueue(), func(old interface{}, new interface{}) bool {
		oldObj := old.(*api.MongodbRole)
		newObj := new.(*api.MongodbRole)
		return newObj.DeletionTimestamp != nil || !newObj.AlreadyObserved(oldObj)
	}))
	c.mongodbRoleLister = c.dbInformerFactory.Authorization().V1alpha1().MongodbRoles().Lister()
}

func (c *UserManagerController) runMongodbRoleInjector(key string) error {
	obj, exist, err := c.mgRoleInformer.GetIndexer().GetByKey(key)
	if err != nil {
		glog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exist {
		glog.Warningf("MongodbRole %s does not exist anymore\n", key)

	} else {
		mRole := obj.(*api.MongodbRole).DeepCopy()

		glog.Infof("Sync/Add/Update for MongodbRole %s/%s\n", mRole.Namespace, mRole.Name)

		if mRole.DeletionTimestamp != nil {
			if kutilcorev1.HasFinalizer(mRole.ObjectMeta, MongodbRoleFinalizer) {
				go c.runMongodbRoleFinalizer(mRole, 1*time.Minute, 10*time.Second)
			}
		} else {
			if !kutilcorev1.HasFinalizer(mRole.ObjectMeta, MongodbRoleFinalizer) {
				// Add finalizer
				_, _, err := patchutil.PatchMongodbRole(c.dbClient.AuthorizationV1alpha1(), mRole, func(role *api.MongodbRole) *api.MongodbRole {
					role.ObjectMeta = kutilcorev1.AddFinalizer(role.ObjectMeta, MongodbRoleFinalizer)
					return role
				})
				if err != nil {
					return errors.Wrapf(err, "failed to set MongodbRole finalizer for (%s/%s)", mRole.Namespace, mRole.Name)
				}
			}
			dbRClient, err := database.NewDatabaseRoleForMongodb(c.kubeClient, mRole)
			if err != nil {
				return err
			}

			err = c.reconcileMongodbRole(dbRClient, mRole)
			if err != nil {
				return err
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
func (c *UserManagerController) reconcileMongodbRole(dbRClient database.DatabaseRoleInterface, mRole *api.MongodbRole) error {
	if mRole.Status.Phase == "" { // initial stage
		status := mRole.Status

		// enable the database secrets engine if it is not already enabled
		err := dbRClient.EnableDatabase()
		if err != nil {
			status.Conditions = []api.MongodbRoleCondition{
				{
					Type:    "Available",
					Status:  corev1.ConditionFalse,
					Reason:  "FailedToEnableDatabase",
					Message: err.Error(),
				},
			}

			err2 := c.updatedMongodbRoleStatus(&status, mRole)
			if err2 != nil {
				return errors.Wrapf(err2, "for MongodbRole(%s/%s): failed to update status", mRole.Namespace, mRole.Name)
			}

			return errors.Wrapf(err, "For MongodbRole(%s/%s): failed to enable database secret engine", mRole.Namespace, mRole.Name)
		}

		// create database config for Mongodb
		err = dbRClient.CreateConfig()
		if err != nil {
			status.Conditions = []api.MongodbRoleCondition{
				{
					Type:    "Available",
					Status:  corev1.ConditionFalse,
					Reason:  "FailedToCreateDatabaseConfig",
					Message: err.Error(),
				},
			}

			err2 := c.updatedMongodbRoleStatus(&status, mRole)
			if err2 != nil {
				return errors.Wrapf(err2, "for MongodbRole(%s/%s): failed to update status", mRole.Namespace, mRole.Name)
			}

			return errors.Wrapf(err, "For MongodbRole(%s/%s): failed to created database connection config(%s)", mRole.Namespace, mRole.Name, mRole.Spec.Database.Name)
		}

		// create role
		err = dbRClient.CreateRole()
		if err != nil {
			status.Conditions = []api.MongodbRoleCondition{
				{
					Type:    "Available",
					Status:  corev1.ConditionFalse,
					Reason:  "FailedToCreateRole",
					Message: err.Error(),
				},
			}

			err2 := c.updatedMongodbRoleStatus(&status, mRole)
			if err2 != nil {
				return errors.Wrapf(err2, "for MongodbRole(%s/%s): failed to update status", mRole.Namespace, mRole.Name)
			}

			return errors.Wrapf(err, "For MongodbRole(%s/%s): failed to create role", mRole.Namespace, mRole.Name)
		}

		status.Conditions = []api.MongodbRoleCondition{}
		status.Phase = MongodbRolePhaseSuccess
		status.ObservedGeneration = mRole.Generation

		err = c.updatedMongodbRoleStatus(&status, mRole)
		if err != nil {
			return errors.Wrapf(err, "For MongodbRole(%s/%s): failed to update MongodbRole status", mRole.Namespace, mRole.Name)
		}

	} else {
		// sync role
		if mRole.ObjectMeta.Generation > mRole.Status.ObservedGeneration {
			status := mRole.Status

			// In vault create role replaces the old role
			err := dbRClient.CreateRole()
			if err != nil {
				status.Conditions = []api.MongodbRoleCondition{
					{
						Type:    "Available",
						Status:  corev1.ConditionFalse,
						Reason:  "FailedToUpdateRole",
						Message: err.Error(),
					},
				}

				err2 := c.updatedMongodbRoleStatus(&status, mRole)
				if err2 != nil {
					return errors.Wrapf(err2, "for MongodbRole(%s/%s): failed to update status", mRole.Namespace, mRole.Name)
				}

				return errors.Wrapf(err, "For Mongodb(%s/%s): failed to update role", mRole.Namespace, mRole.Name)
			}

			status.Conditions = []api.MongodbRoleCondition{}
			status.ObservedGeneration = mRole.Generation

			err = c.updatedMongodbRoleStatus(&status, mRole)
			if err != nil {
				return errors.Wrapf(err, "For Mongodb(%s/%s): failed to update MongodbRole status", mRole.Namespace, mRole.Name)
			}
		}
	}

	return nil
}

func (c *UserManagerController) updatedMongodbRoleStatus(status *api.MongodbRoleStatus, mRole *api.MongodbRole) error {
	_, err := patchutil.UpdateMongodbRoleStatus(c.dbClient.AuthorizationV1alpha1(), mRole, func(s *api.MongodbRoleStatus) *api.MongodbRoleStatus {
		s = status
		return s
	})
	if err != nil {
		return err
	}

	return nil
}

func (c *UserManagerController) runMongodbRoleFinalizer(mRole *api.MongodbRole, timeout time.Duration, interval time.Duration) {
	id := getMongodbRoleId(mRole)

	if _, ok := c.processingFinalizer[id]; ok {
		// already processing
		return
	}

	c.processingFinalizer[id] = true

	stopCh := time.After(timeout)
	finalizationDone := false

	for {
		m, err := c.dbClient.AuthorizationV1alpha1().MongodbRoles(mRole.Namespace).Get(mRole.Name, metav1.GetOptions{})
		if kerr.IsNotFound(err) {
			delete(c.processingFinalizer, id)
			return
		} else if err != nil {
			glog.Errorf("MongodbRole(%s/%s) finalizer: %v\n", mRole.Namespace, mRole.Name, err)
		}

		// to make sure p is not nil
		if m == nil {
			m = mRole
		}

		select {
		case <-stopCh:
			err := c.removeMongodbRoleFinalizer(m)
			if err != nil {
				glog.Errorf("MongodbRole(%s/%s) finalizer: %v\n", m.Namespace, m.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		default:
		}

		if !finalizationDone {
			d, err := database.NewDatabaseRoleForMongodb(c.kubeClient, m)
			if err != nil {
				glog.Errorf("MongodbRole(%s/%s) finalizer: %v\n", m.Namespace, m.Name, err)
			} else {
				err = c.finalizeMongodbRole(d, m)
				if err != nil {
					glog.Errorf("MongodbRole(%s/%s) finalizer: %v\n", m.Namespace, m.Name, err)
				} else {
					finalizationDone = true
				}
			}

		}

		if finalizationDone {
			err := c.removeMongodbRoleFinalizer(m)
			if err != nil {
				glog.Errorf("MongodbRole(%s/%s) finalizer: %v\n", m.Namespace, m.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		}

		select {
		case <-stopCh:
			err := c.removeMongodbRoleFinalizer(m)
			if err != nil {
				glog.Errorf("MongodbRole(%s/%s) finalizer: %v\n", m.Namespace, m.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		case <-time.After(interval):
		}
	}
}

func (c *UserManagerController) finalizeMongodbRole(dbRClient database.DatabaseRoleInterface, mRole *api.MongodbRole) error {
	err := dbRClient.DeleteRole(mRole.Name)
	if err != nil {
		return errors.Wrap(err, "failed to database role")
	}

	return nil
}

func (c *UserManagerController) removeMongodbRoleFinalizer(mRole *api.MongodbRole) error {
	// remove finalizer
	_, _, err := patchutil.PatchMongodbRole(c.dbClient.AuthorizationV1alpha1(), mRole, func(role *api.MongodbRole) *api.MongodbRole {
		role.ObjectMeta = kutilcorev1.RemoveFinalizer(role.ObjectMeta, MongodbRoleFinalizer)
		return role
	})
	if err != nil {
		return err
	}

	return nil
}

func getMongodbRoleId(mRole *api.MongodbRole) string {
	return fmt.Sprintf("%s/%s/%s", api.ResourceMongodbRole, mRole.Namespace, mRole.Name)
}
