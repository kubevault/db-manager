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
	MysqlRoleBindingFinalizer = "database.mysql.rolebinding"
)

const (
	MysqlRoleBindingPhaseSuccess           api.MysqlRoleBindingPhase = "Success"
	MysqlRoleBindingPhaseInit              api.MysqlRoleBindingPhase = "Init"
	MysqlRoleBindingPhaseGetCredential     api.MysqlRoleBindingPhase = "GetCredential"
	MysqlRoleBindingPhaseCreateSecret      api.MysqlRoleBindingPhase = "CreateSecret"
	MysqlRoleBindingPhaseCreateRole        api.MysqlRoleBindingPhase = "CreateRole"
	MysqlRoleBindingPhaseCreateRoleBinding api.MysqlRoleBindingPhase = "CreateRoleBinding"
)

func (c *UserManagerController) initMysqlRoleBindingWatcher() {
	c.myRoleBindingInformer = c.dbInformerFactory.Authorization().V1alpha1().MysqlRoleBindings().Informer()
	c.myRoleBindingQueue = queue.New(api.ResourceKindMysqlRoleBinding, c.MaxNumRequeues, c.NumThreads, c.runMysqlRoleBindingInjector)

	// TODO: add custom event handler?
	c.myRoleBindingInformer.AddEventHandler(queue.DefaultEventHandler(c.myRoleBindingQueue.GetQueue()))
	c.myRoleBindingLister = c.dbInformerFactory.Authorization().V1alpha1().MysqlRoleBindings().Lister()
}

func (c *UserManagerController) runMysqlRoleBindingInjector(key string) error {
	obj, exist, err := c.myRoleBindingInformer.GetIndexer().GetByKey(key)
	if err != nil {
		glog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exist {
		glog.Warningf("MysqlRoleBinding %s does not exist anymore\n", key)

	} else {
		mRoleBinding := obj.(*api.MysqlRoleBinding)

		glog.Infof("Sync/Add/Update for MysqlRoleBinding %s/%s\n", mRoleBinding.Namespace, mRoleBinding.Name)

		if mRoleBinding.DeletionTimestamp != nil {
			if kutilcorev1.HasFinalizer(mRoleBinding.ObjectMeta, MysqlRoleBindingFinalizer) {
				go c.runMysqlRoleBindingFinalizer(mRoleBinding, 1*time.Minute, 10*time.Second)
			}

		} else if !kutilcorev1.HasFinalizer(mRoleBinding.ObjectMeta, MysqlRoleBindingFinalizer) {
			// Add finalizer
			_, _, err = patchutil.PatchMysqlRoleBinding(c.dbClient.AuthorizationV1alpha1(), mRoleBinding, func(binding *api.MysqlRoleBinding) *api.MysqlRoleBinding {
				binding.ObjectMeta = kutilcorev1.AddFinalizer(binding.ObjectMeta, MysqlRoleBindingFinalizer)
				return binding
			})
			if err != nil {
				return errors.Wrapf(err, "failed to set MysqlRoleBinding finalizer for (%s/%s)", mRoleBinding.Namespace, mRoleBinding.Name)
			}

		} else {
			dbRBClient, err := database.NewDatabaseRoleBindingForMysql(c.kubeClient, c.dbClient, mRoleBinding)
			if err != nil {
				return err
			}

			err = c.reconcileMysqlRoleBinding(dbRBClient, mRoleBinding)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Will do:
//	For vault:
//	  - get Mysql credential
//	  - create secret containing credential
//	  - create rbac role and role binding
//    - sync role binding
func (c *UserManagerController) reconcileMysqlRoleBinding(dbRBClient database.DatabaseRoleBindingInterface, mRoleBinding *api.MysqlRoleBinding) error {
	if mRoleBinding.Status.ObservedGeneration == 0 { // initial stage
		var (
			cred *vault.DatabaseCredential
			err  error
		)

		status := mRoleBinding.Status
		name := mRoleBinding.Name
		namespace := mRoleBinding.Namespace
		roleName := getMysqlRbacRoleName(name)
		roleBindingName := getMysqlRbacRoleBindingName(name)
		storeSecret := mRoleBinding.Spec.Store.Secret

		if status.Phase == "" || status.Phase == MysqlRoleBindingPhaseGetCredential || status.Phase == MysqlRoleBindingPhaseCreateSecret {
			status.Phase = MysqlRoleBindingPhaseGetCredential

			cred, err = dbRBClient.GetCredential()
			if err != nil {
				status.Conditions = []api.MysqlRoleBindingCondition{
					{
						Type:    "Available",
						Status:  corev1.ConditionFalse,
						Reason:  "FailedToGetCredential",
						Message: err.Error(),
					},
				}

				err2 := c.updateMysqlRoleBindingStatus(&status, mRoleBinding)
				if err2 != nil {
					return errors.Wrapf(err2, "for MysqlRoleBinding(%s/%s): failed to update status", namespace, name)
				}

				return errors.Wrapf(err, "for MysqlRoleBinding(%s/%s)", namespace, name)
			}

			glog.Infof("for MysqlRoleBinding(%s/%s): getting Mysql credential is successful\n", namespace, name)

			// add lease info
			d := time.Duration(cred.LeaseDuration)
			status.Lease = api.LeaseData{
				ID:            cred.LeaseID,
				Duration:      cred.LeaseDuration,
				RenewDeadline: time.Now().Add(time.Second * d).Unix(),
			}

			// next phase
			status.Phase = MysqlRoleBindingPhaseCreateSecret
		}

		if status.Phase == MysqlRoleBindingPhaseCreateSecret {
			err = dbRBClient.CreateSecret(storeSecret, namespace, cred)
			if err != nil {
				err2 := dbRBClient.RevokeLease(cred.LeaseID)
				if err2 != nil {
					return errors.Wrapf(err2, "for MysqlRoleBinding(%s/%s): failed to revoke lease", namespace, name)
				}

				status.Conditions = []api.MysqlRoleBindingCondition{
					{
						Type:    "Available",
						Status:  corev1.ConditionFalse,
						Reason:  "FailedToCreateSecret",
						Message: err.Error(),
					},
				}

				err2 = c.updateMysqlRoleBindingStatus(&status, mRoleBinding)
				if err2 != nil {
					return errors.Wrapf(err2, "for MysqlRoleBinding(%s/%s): failed to update status", namespace, name)
				}

				return errors.Wrapf(err, "for MysqlRoleBinding(%s/%s)", namespace, name)
			}
			glog.Infof("for MysqlRoleBinding(%s/%s): creating secret(%s/%s) is successful\n", namespace, name, namespace, mRoleBinding.Spec.Store.Secret)

			// next phase
			status.Phase = MysqlRoleBindingPhaseCreateRole
		}

		if status.Phase == MysqlRoleBindingPhaseCreateRole {
			err = dbRBClient.CreateRole(roleName, namespace, storeSecret)
			if err != nil {
				status.Conditions = []api.MysqlRoleBindingCondition{
					{
						Type:    "Available",
						Status:  corev1.ConditionFalse,
						Reason:  "FailedToCreateRole",
						Message: err.Error(),
					},
				}

				err2 := c.updateMysqlRoleBindingStatus(&status, mRoleBinding)
				if err2 != nil {
					return errors.Wrapf(err2, "for MysqlRoleBinding(%s/%s): failed to update status", namespace, name)
				}

				return errors.Wrapf(err, "for MysqlRoleBinding(%s/%s)", namespace, name)
			}
			glog.Infof("for MysqlRoleBinding(%s/%s): creating rbac role(%s/%s) is successful\n", namespace, name, namespace, roleName)

			//next phase
			status.Phase = MysqlRoleBindingPhaseCreateRoleBinding
		}

		if status.Phase == MysqlRoleBindingPhaseCreateRoleBinding {
			err = dbRBClient.CreateRoleBinding(roleBindingName, namespace, roleName, mRoleBinding.Spec.Subjects)
			if err != nil {
				status.Conditions = []api.MysqlRoleBindingCondition{
					{
						Type:    "Available",
						Status:  corev1.ConditionFalse,
						Reason:  "FailedToCreateRoleBinding",
						Message: err.Error(),
					},
				}

				err2 := c.updateMysqlRoleBindingStatus(&status, mRoleBinding)
				if err2 != nil {
					return errors.Wrapf(err2, "for MysqlRoleBinding(%s/%s): failed to update status", namespace, name)
				}

				return errors.Wrapf(err, "for MysqlRoleBinding(%s/%s)", namespace, name)
			}
			glog.Infof("for MysqlRoleBinding(%s/%s): creating rbac role binding(%s/%s) is successful\n", namespace, name, namespace, roleBindingName)
		}

		status.Phase = MysqlRoleBindingPhaseSuccess
		status.Conditions = []api.MysqlRoleBindingCondition{}
		status.ObservedGeneration = mRoleBinding.GetGeneration()

		err = c.updateMysqlRoleBindingStatus(&status, mRoleBinding)
		if err != nil {
			return errors.Wrapf(err, "for MysqlRoleBinding(%s/%s): failed to update status", namespace, name)
		}

	} else {
		// sync role binding
		// - update role binding
		if mRoleBinding.ObjectMeta.Generation > mRoleBinding.Status.ObservedGeneration {
			name := mRoleBinding.Name
			namespace := mRoleBinding.Namespace
			status := mRoleBinding.Status

			err := dbRBClient.UpdateRoleBinding(getMysqlRbacRoleBindingName(name), namespace, mRoleBinding.Spec.Subjects)
			if err != nil {
				status.Conditions = []api.MysqlRoleBindingCondition{
					{
						Type:    "Available",
						Status:  corev1.ConditionFalse,
						Reason:  "FailedToUpdateRoleBinding",
						Message: err.Error(),
					},
				}

				err2 := c.updateMysqlRoleBindingStatus(&status, mRoleBinding)
				if err2 != nil {
					return errors.Wrapf(err2, "for mysqlRoleBinding(%s/%s): failed to update status", namespace, name)
				}

				return errors.Wrapf(err, "for MysqlRoleBinding(%s/%s)", namespace, name)
			}

			status.Conditions = []api.MysqlRoleBindingCondition{}
			status.ObservedGeneration = mRoleBinding.ObjectMeta.Generation

			err = c.updateMysqlRoleBindingStatus(&status, mRoleBinding)
			if err != nil {
				return errors.Wrapf(err, "for MysqlRoleBinding(%s/%s)", namespace, name)
			}
		}
	}

	return nil
}

func (c *UserManagerController) updateMysqlRoleBindingStatus(status *api.MysqlRoleBindingStatus, mRoleBinding *api.MysqlRoleBinding) error {
	_, err := patchutil.UpdateMysqlRoleBindingStatus(c.dbClient.AuthorizationV1alpha1(), mRoleBinding, func(s *api.MysqlRoleBindingStatus) *api.MysqlRoleBindingStatus {
		s = status
		return s
	})
	if err != nil {
		return err
	}

	return nil
}

func (c *UserManagerController) runMysqlRoleBindingFinalizer(mRoleBinding *api.MysqlRoleBinding, timeout time.Duration, interval time.Duration) {
	id := getMysqlRoleBindingId(mRoleBinding)

	if _, ok := c.processingFinalizer[id]; ok {
		// already processing
		return
	}

	c.processingFinalizer[id] = true

	stopCh := time.After(timeout)
	finalizationDone := false

	for {
		m, err := c.dbClient.AuthorizationV1alpha1().MysqlRoleBindings(mRoleBinding.Namespace).Get(mRoleBinding.Name, metav1.GetOptions{})
		if kerr.IsNotFound(err) {
			delete(c.processingFinalizer, id)
			return
		} else if err != nil {
			glog.Errorf("MysqlRoleBinding(%s/%s) finalizer: %v\n", mRoleBinding.Namespace, mRoleBinding.Name, err)
		}

		// to make sure m is not nil
		if m == nil {
			m = mRoleBinding
		}

		select {
		case <-stopCh:
			err := c.removeMysqlRoleBindingFinalizer(m)
			if err != nil {
				glog.Errorf("MysqlRoleBinding(%s/%s) finalizer: %v\n", m.Namespace, m.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		default:
		}

		if !finalizationDone {
			d, err := database.NewDatabaseRoleBindingForMysql(c.kubeClient, c.dbClient, m)
			if err != nil {
				glog.Errorf("MysqlRoleBinding(%s/%s) finalizer: %v\n", m.Namespace, m.Name, err)
			} else {
				err = c.finalizeMysqlRoleBinding(d, m.Status.Lease.ID)
				if err != nil {
					glog.Errorf("MysqlRoleBinding(%s/%s) finalizer: %v\n", m.Namespace, m.Name, err)
				} else {
					finalizationDone = true
				}
			}
		}

		if finalizationDone {
			err := c.removeMysqlRoleBindingFinalizer(m)
			if err != nil {
				glog.Errorf("MysqlRoleBinding(%s/%s) finalizer: %v\n", m.Namespace, m.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		}

		select {
		case <-stopCh:
			err := c.removeMysqlRoleBindingFinalizer(m)
			if err != nil {
				glog.Errorf("MysqlRoleBinding(%s/%s) finalizer: %v\n", m.Namespace, m.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		case <-time.After(interval):
		}
	}
}

func (c *UserManagerController) finalizeMysqlRoleBinding(dbRBClient database.DatabaseRoleBindingInterface, leaseID string) error {
	if leaseID == "" {
		return nil
	}

	err := dbRBClient.RevokeLease(leaseID)
	if err != nil {
		return err
	}
	return nil
}

func (c *UserManagerController) removeMysqlRoleBindingFinalizer(mRoleBinding *api.MysqlRoleBinding) error {
	_, _, err := patchutil.PatchMysqlRoleBinding(c.dbClient.AuthorizationV1alpha1(), mRoleBinding, func(r *api.MysqlRoleBinding) *api.MysqlRoleBinding {
		r.ObjectMeta = kutilcorev1.RemoveFinalizer(r.ObjectMeta, MysqlRoleBindingFinalizer)
		return r
	})
	if err != nil {
		return err
	}

	return nil
}

func getMysqlRoleBindingId(mRoleBinding *api.MysqlRoleBinding) string {
	return fmt.Sprintf("%s/%s/%s", api.ResourceMysqlRoleBinding, mRoleBinding.Namespace, mRoleBinding.Name)
}

func getMysqlRbacRoleName(name string) string {
	return fmt.Sprintf("mysqlrolebinding-%s-credential-reader", name)
}

func getMysqlRbacRoleBindingName(name string) string {
	return fmt.Sprintf("mysqlrolebinding-%s-credential-reader", name)
}
