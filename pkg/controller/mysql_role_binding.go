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
	MysqlRoleBindingFinalizer = "database.mysql.rolebinding"
)

func (c *UserManagerController) initMysqlRoleBindingWatcher() {
	c.myRoleBindingInformer = c.dbInformerFactory.Authorization().V1alpha1().MysqlRoleBindings().Informer()
	c.myRoleBindingQueue = queue.New(api.ResourceKindMysqlRoleBinding, c.MaxNumRequeues, c.NumThreads, c.runMysqlRoleBindingInjector)
	c.myRoleBindingInformer.AddEventHandler(queue.NewEventHandler(c.myRoleBindingQueue.GetQueue(), func(old interface{}, new interface{}) bool {
		oldObj := old.(*api.MysqlRoleBinding)
		newObj := new.(*api.MysqlRoleBinding)
		return newObj.DeletionTimestamp != nil || !newObj.AlreadyObserved(oldObj)
	}))
	c.myRoleBindingLister = c.dbInformerFactory.Authorization().V1alpha1().MysqlRoleBindings().Lister()
}

func (c *UserManagerController) runMysqlRoleBindingInjector(key string) error {
	obj, exist, err := c.myRoleBindingInformer.GetIndexer().GetByKey(key)
	if err != nil {
		glog.Errorf("Fetching object with key(%s) from store failed with %v", key, err)
		return err
	}

	if !exist {
		glog.Warningf("MysqlRoleBinding(%s) does not exist anymore\n", key)

	} else {
		mRoleBinding := obj.(*api.MysqlRoleBinding).DeepCopy()

		glog.Infof("Sync/Add/Update for MysqlRoleBinding(%s/%s)\n", mRoleBinding.Namespace, mRoleBinding.Name)

		if mRoleBinding.DeletionTimestamp != nil {
			if kutilcorev1.HasFinalizer(mRoleBinding.ObjectMeta, MysqlRoleBindingFinalizer) {
				go c.runMysqlRoleBindingFinalizer(mRoleBinding, 1*time.Minute, 10*time.Second)
			}

		} else {
			if !kutilcorev1.HasFinalizer(mRoleBinding.ObjectMeta, MysqlRoleBindingFinalizer) {
				// Add finalizer
				_, _, err = patchutil.PatchMysqlRoleBinding(c.dbClient.AuthorizationV1alpha1(), mRoleBinding, func(binding *api.MysqlRoleBinding) *api.MysqlRoleBinding {
					binding.ObjectMeta = kutilcorev1.AddFinalizer(binding.ObjectMeta, MysqlRoleBindingFinalizer)
					return binding
				})
				if err != nil {
					return errors.Wrapf(err, "failed to set MysqlRoleBinding finalizer for (%s/%s)", mRoleBinding.Namespace, mRoleBinding.Name)
				}

			}

			dbRBClient, err := database.NewDatabaseRoleBindingForMysql(c.kubeClient, c.dbClient, mRoleBinding)
			if err != nil {
				return err
			}

			err = c.reconcileMysqlRoleBinding(dbRBClient, mRoleBinding)
			if err != nil {
				return errors.Wrapf(err, "For MysqlRoleBinding(%s/%s)", mRoleBinding.Namespace, mRoleBinding.Name)
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
	var (
		err   error
		credS *corev1.Secret
	)

	var (
		mRBName    = mRoleBinding.Name
		ns         = mRoleBinding.Namespace
		secretName = mRoleBinding.Spec.Store.Secret
		status     = mRoleBinding.Status
	)

	// get credential secret. if not found, then create it
	credS, err = c.kubeClient.CoreV1().Secrets(ns).Get(secretName, metav1.GetOptions{})
	if err != nil && !kerr.IsNotFound(err) {
		return errors.WithStack(err)
	}

	// is lease_id exists in credential secret
	// if it exists, then is it expired
	isLeaseExpired := true
	if credS != nil && credS.Data != nil {
		leaseID, ok := credS.Data["lease_id"]
		if ok {
			isLeaseExpired, err = dbRBClient.IsLeaseExpired(string(leaseID))
			if err != nil {
				return errors.WithStack(err)
			}
		}
	}

	if isLeaseExpired {
		// get database credential
		cred, err := dbRBClient.GetCredential()
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
				return errors.Wrapf(err2, "failed to update status")
			}
			return errors.WithStack(err)
		}

		err = dbRBClient.CreateSecret(secretName, ns, cred)
		if err != nil {
			err2 := dbRBClient.RevokeLease(cred.LeaseID)
			if err2 != nil {
				return errors.Wrapf(err2, "failed to revoke lease")
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
				return errors.Wrapf(err2, "failed to update status")
			}
			return errors.WithStack(err)
		}

		// add lease info in status
		status.Lease = api.LeaseData{
			ID:            cred.LeaseID,
			Duration:      cred.LeaseDuration,
			RenewDeadline: time.Now().Unix(),
		}
	}

	err = dbRBClient.CreateRole(getMysqlRoleName(mRBName), ns, secretName)
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
			return errors.Wrapf(err2, "failed to update status")
		}
		return errors.WithStack(err)
	}

	err = dbRBClient.CreateRoleBinding(getMysqlRoleBindingName(mRBName), ns, getMysqlRoleName(mRBName), mRoleBinding.Spec.Subjects)
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
			return errors.Wrapf(err2, "failed to update status")
		}
		return errors.WithStack(err)
	}

	status.Conditions = []api.MysqlRoleBindingCondition{}
	status.ObservedGeneration = mRoleBinding.Generation

	err = c.updateMysqlRoleBindingStatus(&status, mRoleBinding)
	if err != nil {
		return errors.WithStack(err)
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

func getMysqlRoleName(name string) string {
	return fmt.Sprintf("mysqlrolebinding-%s-credential-reader", name)
}

func getMysqlRoleBindingName(name string) string {
	return fmt.Sprintf("mysqlrolebinding-%s-credential-reader", name)
}
