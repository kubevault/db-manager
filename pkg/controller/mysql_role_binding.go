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

func (c *Controller) initMySQLRoleBindingWatcher() {
	c.myRoleBindingInformer = c.dbInformerFactory.Authorization().V1alpha1().MySQLRoleBindings().Informer()
	c.myRoleBindingQueue = queue.New(api.ResourceKindMySQLRoleBinding, c.MaxNumRequeues, c.NumThreads, c.runMySQLRoleBindingInjector)
	c.myRoleBindingInformer.AddEventHandler(queue.NewObservableHandler(c.myRoleBindingQueue.GetQueue(), apis.EnableStatusSubresource))
	c.myRoleBindingLister = c.dbInformerFactory.Authorization().V1alpha1().MySQLRoleBindings().Lister()
}

func (c *Controller) runMySQLRoleBindingInjector(key string) error {
	obj, exist, err := c.myRoleBindingInformer.GetIndexer().GetByKey(key)
	if err != nil {
		glog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exist {
		glog.Warningf("MySQLRoleBinding %s does not exist anymore", key)

	} else {
		mRoleBinding := obj.(*api.MySQLRoleBinding).DeepCopy()

		glog.Infof("Sync/Add/Update for MySQLRoleBinding %s/%s", mRoleBinding.Namespace, mRoleBinding.Name)

		if mRoleBinding.DeletionTimestamp != nil {
			if core_util.HasFinalizer(mRoleBinding.ObjectMeta, apis.Finalizer) {
				go c.runMySQLRoleBindingFinalizer(mRoleBinding, 1*time.Minute, 10*time.Second)
			}

		} else {
			if !core_util.HasFinalizer(mRoleBinding.ObjectMeta, apis.Finalizer) {
				// Add finalizer
				_, _, err = patchutil.PatchMySQLRoleBinding(c.dbClient.AuthorizationV1alpha1(), mRoleBinding, func(binding *api.MySQLRoleBinding) *api.MySQLRoleBinding {
					binding.ObjectMeta = core_util.AddFinalizer(binding.ObjectMeta, apis.Finalizer)
					return binding
				})
				if err != nil {
					return errors.Wrapf(err, "failed to set MySQLRoleBinding finalizer for %s/%s", mRoleBinding.Namespace, mRoleBinding.Name)
				}

			}

			dbRBClient, err := database.NewDatabaseRoleBindingForMysql(c.kubeClient, c.catalogClient.AppcatalogV1alpha1(), c.dbClient, mRoleBinding)
			if err != nil {
				return err
			}

			err = c.reconcileMySQLRoleBinding(dbRBClient, mRoleBinding)
			if err != nil {
				return errors.Wrapf(err, "For MySQLRoleBinding %s/%s", mRoleBinding.Namespace, mRoleBinding.Name)
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
func (c *Controller) reconcileMySQLRoleBinding(dbRBClient database.DatabaseRoleBindingInterface, myRoleBinding *api.MySQLRoleBinding) error {
	var (
		err   error
		credS *corev1.Secret
	)

	var (
		myRBName   = myRoleBinding.Name
		ns         = myRoleBinding.Namespace
		secretName = myRoleBinding.Spec.Store.Secret
		status     = myRoleBinding.Status
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
			status.Conditions = []api.MySQLRoleBindingCondition{
				{
					Type:    "Available",
					Status:  corev1.ConditionFalse,
					Reason:  "FailedToGetCredential",
					Message: err.Error(),
				},
			}

			err2 := c.updateMySQLRoleBindingStatus(&status, myRoleBinding)
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

			status.Conditions = []api.MySQLRoleBindingCondition{
				{
					Type:    "Available",
					Status:  corev1.ConditionFalse,
					Reason:  "FailedToCreateSecret",
					Message: err.Error(),
				},
			}

			err2 = c.updateMySQLRoleBindingStatus(&status, myRoleBinding)
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

	err = dbRBClient.CreateRole(getMySQLRoleName(myRBName), ns, secretName)
	if err != nil {
		status.Conditions = []api.MySQLRoleBindingCondition{
			{
				Type:    "Available",
				Status:  corev1.ConditionFalse,
				Reason:  "FailedToCreateRole",
				Message: err.Error(),
			},
		}

		err2 := c.updateMySQLRoleBindingStatus(&status, myRoleBinding)
		if err2 != nil {
			return errors.Wrapf(err2, "failed to update status")
		}
		return errors.WithStack(err)
	}

	err = dbRBClient.CreateRoleBinding(getMySQLRoleBindingName(myRBName), ns, getMySQLRoleName(myRBName), myRoleBinding.Spec.Subjects)
	if err != nil {
		status.Conditions = []api.MySQLRoleBindingCondition{
			{
				Type:    "Available",
				Status:  corev1.ConditionFalse,
				Reason:  "FailedToCreateRoleBinding",
				Message: err.Error(),
			},
		}

		err2 := c.updateMySQLRoleBindingStatus(&status, myRoleBinding)
		if err2 != nil {
			return errors.Wrapf(err2, "failed to update status")
		}
		return errors.WithStack(err)
	}

	status.Conditions = []api.MySQLRoleBindingCondition{}
	status.ObservedGeneration = types.NewIntHash(myRoleBinding.Generation, meta_util.GenerationHash(myRoleBinding))

	err = c.updateMySQLRoleBindingStatus(&status, myRoleBinding)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (c *Controller) updateMySQLRoleBindingStatus(status *api.MySQLRoleBindingStatus, myRoleBinding *api.MySQLRoleBinding) error {
	_, err := patchutil.UpdateMySQLRoleBindingStatus(c.dbClient.AuthorizationV1alpha1(), myRoleBinding, func(s *api.MySQLRoleBindingStatus) *api.MySQLRoleBindingStatus {
		s = status
		return s
	})
	if err != nil {
		return err
	}
	return nil
}

func (c *Controller) runMySQLRoleBindingFinalizer(mRoleBinding *api.MySQLRoleBinding, timeout time.Duration, interval time.Duration) {
	id := getMySQLRoleBindingId(mRoleBinding)

	if _, ok := c.processingFinalizer[id]; ok {
		// already processing
		return
	}

	c.processingFinalizer[id] = true
	glog.Infof("MySQLRoleBinding %s/%s finalizer: start processing\n", mRoleBinding.Namespace, mRoleBinding.Name)

	stopCh := time.After(timeout)
	finalizationDone := false
	attempt := 0

	for {
		m, err := c.dbClient.AuthorizationV1alpha1().MySQLRoleBindings(mRoleBinding.Namespace).Get(mRoleBinding.Name, metav1.GetOptions{})
		if kerr.IsNotFound(err) {
			delete(c.processingFinalizer, id)
			return
		} else if err != nil {
			glog.Errorf("MySQLRoleBinding %s/%s finalizer: %v", mRoleBinding.Namespace, mRoleBinding.Name, err)
		}

		// to make sure m is not nil
		if m == nil {
			m = mRoleBinding
		}

		select {
		case <-stopCh:
			err := c.removeMySQLRoleBindingFinalizer(m)
			if err != nil {
				glog.Errorf("MySQLRoleBinding %s/%s finalizer: %v", m.Namespace, m.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		default:
		}

		if !finalizationDone {
			d, err := database.NewDatabaseRoleBindingForMysql(c.kubeClient, c.catalogClient.AppcatalogV1alpha1(), c.dbClient, m)
			if err != nil {
				glog.Errorf("MySQLRoleBinding %s/%s finalizer: %v", m.Namespace, m.Name, err)
			} else {
				err = c.finalizeMySQLRoleBinding(d, m.Status.Lease.ID)
				if err != nil {
					glog.Errorf("MySQLRoleBinding %s/%s finalizer: %v", m.Namespace, m.Name, err)
				} else {
					finalizationDone = true
				}
			}
		}

		glog.Infof("MySQLRoleBinding %s/%s finalizer: attempt %d\n", mRoleBinding.Namespace, mRoleBinding.Name, attempt)

		if finalizationDone {
			err := c.removeMySQLRoleBindingFinalizer(m)
			if err != nil {
				glog.Errorf("MySQLRoleBinding %s/%s finalizer: %v", m.Namespace, m.Name, err)
			} else {
				delete(c.processingFinalizer, id)
				return
			}
		}

		select {
		case <-stopCh:
			err := c.removeMySQLRoleBindingFinalizer(m)
			if err != nil {
				glog.Errorf("MySQLRoleBinding %s/%s finalizer: %v", m.Namespace, m.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		case <-time.After(interval):
		}
		attempt++
	}
}

func (c *Controller) finalizeMySQLRoleBinding(dbRBClient database.DatabaseRoleBindingInterface, leaseID string) error {
	if leaseID == "" {
		return nil
	}

	err := dbRBClient.RevokeLease(leaseID)
	if err != nil {
		return err
	}
	return nil
}

func (c *Controller) removeMySQLRoleBindingFinalizer(mRoleBinding *api.MySQLRoleBinding) error {
	_, _, err := patchutil.PatchMySQLRoleBinding(c.dbClient.AuthorizationV1alpha1(), mRoleBinding, func(r *api.MySQLRoleBinding) *api.MySQLRoleBinding {
		r.ObjectMeta = core_util.RemoveFinalizer(r.ObjectMeta, apis.Finalizer)
		return r
	})
	if err != nil {
		return err
	}
	return nil
}

func getMySQLRoleBindingId(mRoleBinding *api.MySQLRoleBinding) string {
	return fmt.Sprintf("%s/%s/%s", api.ResourceMySQLRoleBinding, mRoleBinding.Namespace, mRoleBinding.Name)
}

func getMySQLRoleName(name string) string {
	return fmt.Sprintf("mysqlrolebinding-%s-credential-reader", name)
}

func getMySQLRoleBindingName(name string) string {
	return fmt.Sprintf("mysqlrolebinding-%s-credential-reader", name)
}
