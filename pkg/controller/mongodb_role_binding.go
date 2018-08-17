package controller

import (
	"fmt"
	"time"

	kutilcorev1 "github.com/appscode/kutil/core/v1"
	"github.com/appscode/kutil/tools/queue"
	"github.com/golang/glog"
	api "github.com/kubedb/user-manager/apis/authorization/v1alpha1"
	patchutil "github.com/kubedb/user-manager/client/clientset/versioned/typed/authorization/v1alpha1/util"
	"github.com/kubedb/user-manager/pkg/vault"
	"github.com/kubedb/user-manager/pkg/vault/database"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	MongodbRoleBindingFinalizer = "database.mongodb.rolebinding"
)

const (
	MongodbRoleBindingPhaseSuccess           api.MongodbRoleBindingPhase = "Success"
	MongodbRoleBindingPhaseInit              api.MongodbRoleBindingPhase = "Init"
	MongodbRoleBindingPhaseGetCredential     api.MongodbRoleBindingPhase = "GetCredential"
	MongodbRoleBindingPhaseCreateSecret      api.MongodbRoleBindingPhase = "CreateSecret"
	MongodbRoleBindingPhaseCreateRole        api.MongodbRoleBindingPhase = "CreateRole"
	MongodbRoleBindingPhaseCreateRoleBinding api.MongodbRoleBindingPhase = "CreateRoleBinding"
)

func (c *UserManagerController) initMongodbRoleBindingWatcher() {
	c.mgRoleBindingInformer = c.dbInformerFactory.Authorization().V1alpha1().MongodbRoleBindings().Informer()
	c.mgRoleBindingQueue = queue.New(api.ResourceKindMongodbRoleBinding, c.MaxNumRequeues, c.NumThreads, c.runMongodbRoleBindingInjector)

	// TODO: add custom event handler?
	c.mgRoleBindingInformer.AddEventHandler(queue.DefaultEventHandler(c.mgRoleBindingQueue.GetQueue()))
	c.mgRoleBindingLister = c.dbInformerFactory.Authorization().V1alpha1().MongodbRoleBindings().Lister()
}

func (c *UserManagerController) runMongodbRoleBindingInjector(key string) error {
	obj, exist, err := c.mgRoleBindingInformer.GetIndexer().GetByKey(key)
	if err != nil {
		glog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exist {
		glog.Warningf("MongodbRoleBinding %s does not exist anymore\n", key)

	} else {
		mRoleBinding := obj.(*api.MongodbRoleBinding)

		glog.Infof("Sync/Add/Update for MongodbRoleBinding %s/%s\n", mRoleBinding.Namespace, mRoleBinding.Name)

		if mRoleBinding.DeletionTimestamp != nil {
			if kutilcorev1.HasFinalizer(mRoleBinding.ObjectMeta, MongodbRoleBindingFinalizer) {
				go c.runMongodbRoleBindingFinalizer(mRoleBinding, 1*time.Minute, 10*time.Second)
			}

		} else if !kutilcorev1.HasFinalizer(mRoleBinding.ObjectMeta, MongodbRoleBindingFinalizer) {
			// Add finalizer
			_, _, err = patchutil.PatchMongodbRoleBinding(c.dbClient.AuthorizationV1alpha1(), mRoleBinding, func(binding *api.MongodbRoleBinding) *api.MongodbRoleBinding {
				binding.ObjectMeta = kutilcorev1.AddFinalizer(binding.ObjectMeta, MongodbRoleBindingFinalizer)
				return binding
			})
			if err != nil {
				return errors.Wrapf(err, "failed to set MongodbRoleBinding finalizer for (%s/%s)", mRoleBinding.Namespace, mRoleBinding.Name)
			}

		} else {
			dbRBClient, err := database.NewDatabaseRoleBindingForMongodb(c.kubeClient, c.dbClient, mRoleBinding)
			if err != nil {
				return err
			}

			err = c.reconcileMongodbRoleBinding(dbRBClient, mRoleBinding)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Will do:
//	For vault:
//	  - get Mongodb credential
//	  - create secret containing credential
//	  - create rbac role and role binding
//    - sync role binding
func (c *UserManagerController) reconcileMongodbRoleBinding(dbRBClient database.DatabaseRoleBindingInterface, mRoleBinding *api.MongodbRoleBinding) error {
	if mRoleBinding.Status.ObservedGeneration == 0 { // initial stage
		var (
			cred *vault.DatabaseCredential
			err  error
		)

		status := mRoleBinding.Status
		name := mRoleBinding.Name
		namespace := mRoleBinding.Namespace
		roleName := getMongodbRbacRoleName(name)
		roleBindingName := getMongodbRbacRoleBindingName(name)
		storeSecret := mRoleBinding.Spec.Store.Secret

		if status.Phase == "" || status.Phase == MongodbRoleBindingPhaseGetCredential || status.Phase == MongodbRoleBindingPhaseCreateSecret {
			status.Phase = MongodbRoleBindingPhaseGetCredential

			cred, err = dbRBClient.GetCredential()
			if err != nil {
				status.Conditions = []api.MongodbRoleBindingCondition{
					{
						Type:    "Available",
						Status:  corev1.ConditionFalse,
						Reason:  "FailedToGetCredential",
						Message: err.Error(),
					},
				}

				err2 := c.updateMongodbRoleBindingStatus(&status, mRoleBinding)
				if err2 != nil {
					return errors.Wrapf(err2, "for MongodbRoleBinding(%s/%s): failed to update status", namespace, name)
				}

				return errors.Wrapf(err, "for MongodbRoleBinding(%s/%s)", namespace, name)
			}

			glog.Infof("for MongodbRoleBinding(%s/%s): getting Mongodb credential is successful\n", namespace, name)

			// add lease info
			d := time.Duration(cred.LeaseDuration)
			status.Lease = api.LeaseData{
				ID:            cred.LeaseID,
				Duration:      cred.LeaseDuration,
				RenewDeadline: time.Now().Add(time.Second * d).Unix(),
			}

			// next phase
			status.Phase = MongodbRoleBindingPhaseCreateSecret
		}

		if status.Phase == MongodbRoleBindingPhaseCreateSecret {
			err = dbRBClient.CreateSecret(storeSecret, namespace, cred)
			if err != nil {
				err2 := dbRBClient.RevokeLease(cred.LeaseID)
				if err2 != nil {
					return errors.Wrapf(err2, "for MongodbRoleBinding(%s/%s): failed to revoke lease", namespace, name)
				}

				status.Conditions = []api.MongodbRoleBindingCondition{
					{
						Type:    "Available",
						Status:  corev1.ConditionFalse,
						Reason:  "FailedToCreateSecret",
						Message: err.Error(),
					},
				}

				err2 = c.updateMongodbRoleBindingStatus(&status, mRoleBinding)
				if err2 != nil {
					return errors.Wrapf(err2, "for MongodbRoleBinding(%s/%s): failed to update status", namespace, name)
				}

				return errors.Wrapf(err, "for MongodbRoleBinding(%s/%s)", namespace, name)
			}
			glog.Infof("for MongodbRoleBinding(%s/%s): creating secret(%s/%s) is successful\n", namespace, name, namespace, mRoleBinding.Spec.Store.Secret)

			// next phase
			status.Phase = MongodbRoleBindingPhaseCreateRole
		}

		if status.Phase == MongodbRoleBindingPhaseCreateRole {
			err = dbRBClient.CreateRole(roleName, namespace, storeSecret)
			if err != nil {
				status.Conditions = []api.MongodbRoleBindingCondition{
					{
						Type:    "Available",
						Status:  corev1.ConditionFalse,
						Reason:  "FailedToCreateRole",
						Message: err.Error(),
					},
				}

				err2 := c.updateMongodbRoleBindingStatus(&status, mRoleBinding)
				if err2 != nil {
					return errors.Wrapf(err2, "for MongodbRoleBinding(%s/%s): failed to update status", namespace, name)
				}

				return errors.Wrapf(err, "for MongodbRoleBinding(%s/%s)", namespace, name)
			}
			glog.Infof("for MongodbRoleBinding(%s/%s): creating rbac role(%s/%s) is successful\n", namespace, name, namespace, roleName)

			//next phase
			status.Phase = MongodbRoleBindingPhaseCreateRoleBinding
		}

		if status.Phase == MongodbRoleBindingPhaseCreateRoleBinding {
			err = dbRBClient.CreateRoleBinding(roleBindingName, namespace, roleName, mRoleBinding.Spec.Subjects)
			if err != nil {
				status.Conditions = []api.MongodbRoleBindingCondition{
					{
						Type:    "Available",
						Status:  corev1.ConditionFalse,
						Reason:  "FailedToCreateRoleBinding",
						Message: err.Error(),
					},
				}

				err2 := c.updateMongodbRoleBindingStatus(&status, mRoleBinding)
				if err2 != nil {
					return errors.Wrapf(err2, "for MongodbRoleBinding(%s/%s): failed to update status", namespace, name)
				}

				return errors.Wrapf(err, "for MongodbRoleBinding(%s/%s)", namespace, name)
			}
			glog.Infof("for MongodbRoleBinding(%s/%s): creating rbac role binding(%s/%s) is successful\n", namespace, name, namespace, roleBindingName)
		}

		status.Phase = MongodbRoleBindingPhaseSuccess
		status.Conditions = []api.MongodbRoleBindingCondition{}
		status.ObservedGeneration = mRoleBinding.GetGeneration()

		err = c.updateMongodbRoleBindingStatus(&status, mRoleBinding)
		if err != nil {
			return errors.Wrapf(err, "for MongodbRoleBinding(%s/%s): failed to update status", namespace, name)
		}

	} else {
		// sync role binding
		// - update role binding
		if mRoleBinding.ObjectMeta.Generation > mRoleBinding.Status.ObservedGeneration {
			name := mRoleBinding.Name
			namespace := mRoleBinding.Namespace
			status := mRoleBinding.Status

			err := dbRBClient.UpdateRoleBinding(getMongodbRbacRoleBindingName(name), namespace, mRoleBinding.Spec.Subjects)
			if err != nil {
				status.Conditions = []api.MongodbRoleBindingCondition{
					{
						Type:    "Available",
						Status:  corev1.ConditionFalse,
						Reason:  "FailedToUpdateRoleBinding",
						Message: err.Error(),
					},
				}

				err2 := c.updateMongodbRoleBindingStatus(&status, mRoleBinding)
				if err2 != nil {
					return errors.Wrapf(err2, "for mongodbRoleBinding(%s/%s): failed to update status", namespace, name)
				}

				return errors.Wrapf(err, "for MongodbRoleBinding(%s/%s)", namespace, name)
			}

			status.Conditions = []api.MongodbRoleBindingCondition{}
			status.ObservedGeneration = mRoleBinding.ObjectMeta.Generation

			err = c.updateMongodbRoleBindingStatus(&status, mRoleBinding)
			if err != nil {
				return errors.Wrapf(err, "for MongodbRoleBinding(%s/%s)", namespace, name)
			}
		}
	}

	return nil
}

func (c *UserManagerController) updateMongodbRoleBindingStatus(status *api.MongodbRoleBindingStatus, mRoleBinding *api.MongodbRoleBinding) error {
	_, err := patchutil.UpdateMongodbRoleBindingStatus(c.dbClient.AuthorizationV1alpha1(), mRoleBinding, func(s *api.MongodbRoleBindingStatus) *api.MongodbRoleBindingStatus {
		s = status
		return s
	})
	if err != nil {
		return err
	}

	return nil
}

func (c *UserManagerController) runMongodbRoleBindingFinalizer(mRoleBinding *api.MongodbRoleBinding, timeout time.Duration, interval time.Duration) {
	id := getMongodbRoleBindingId(mRoleBinding)

	if _, ok := c.processingFinalizer[id]; ok {
		// already processing
		return
	}

	c.processingFinalizer[id] = true

	stopCh := time.After(timeout)
	finalizationDone := false

	for {
		m, err := c.dbClient.AuthorizationV1alpha1().MongodbRoleBindings(mRoleBinding.Namespace).Get(mRoleBinding.Name, metav1.GetOptions{})
		if kerr.IsNotFound(err) {
			delete(c.processingFinalizer, id)
			return
		} else if err != nil {
			glog.Errorf("MongodbRoleBinding(%s/%s) finalizer: %v\n", mRoleBinding.Namespace, mRoleBinding.Name, err)
		}

		// to make sure m is not nil
		if m == nil {
			m = mRoleBinding
		}

		select {
		case <-stopCh:
			err := c.removeMongodbRoleBindingFinalizer(m)
			if err != nil {
				glog.Errorf("MongodbRoleBinding(%s/%s) finalizer: %v\n", m.Namespace, m.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		default:
		}

		if !finalizationDone {
			d, err := database.NewDatabaseRoleBindingForMongodb(c.kubeClient, c.dbClient, m)
			if err != nil {
				glog.Errorf("MongodbRoleBinding(%s/%s) finalizer: %v\n", m.Namespace, m.Name, err)
			} else {
				err = c.finalizeMongodbRoleBinding(d, m.Status.Lease.ID)
				if err != nil {
					glog.Errorf("MongodbRoleBinding(%s/%s) finalizer: %v\n", m.Namespace, m.Name, err)
				} else {
					finalizationDone = true
				}
			}
		}

		if finalizationDone {
			err := c.removeMongodbRoleBindingFinalizer(m)
			if err != nil {
				glog.Errorf("MongodbRoleBinding(%s/%s) finalizer: %v\n", m.Namespace, m.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		}

		select {
		case <-stopCh:
			err := c.removeMongodbRoleBindingFinalizer(m)
			if err != nil {
				glog.Errorf("MongodbRoleBinding(%s/%s) finalizer: %v\n", m.Namespace, m.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		case <-time.After(interval):
		}
	}
}

func (c *UserManagerController) finalizeMongodbRoleBinding(dbRBClient database.DatabaseRoleBindingInterface, leaseID string) error {
	if leaseID == "" {
		return nil
	}

	err := dbRBClient.RevokeLease(leaseID)
	if err != nil {
		return err
	}
	return nil
}

func (c *UserManagerController) removeMongodbRoleBindingFinalizer(mRoleBinding *api.MongodbRoleBinding) error {
	_, _, err := patchutil.PatchMongodbRoleBinding(c.dbClient.AuthorizationV1alpha1(), mRoleBinding, func(r *api.MongodbRoleBinding) *api.MongodbRoleBinding {
		r.ObjectMeta = kutilcorev1.RemoveFinalizer(r.ObjectMeta, MongodbRoleBindingFinalizer)
		return r
	})
	if err != nil {
		return err
	}

	return nil
}

func getMongodbRoleBindingId(mRoleBinding *api.MongodbRoleBinding) string {
	return fmt.Sprintf("%s/%s/%s", api.ResourceMongodbRoleBinding, mRoleBinding.Namespace, mRoleBinding.Name)
}

func getMongodbRbacRoleName(name string) string {
	return fmt.Sprintf("mongodbrolebinding-%s-credential-reader", name)
}

func getMongodbRbacRoleBindingName(name string) string {
	return fmt.Sprintf("mongodbrolebinding-%s-credential-reader", name)
}
