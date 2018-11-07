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

func (c *Controller) initMongoDBRoleBindingWatcher() {
	c.mgRoleBindingInformer = c.dbInformerFactory.Authorization().V1alpha1().MongoDBRoleBindings().Informer()
	c.mgRoleBindingQueue = queue.New(api.ResourceKindMongoDBRoleBinding, c.MaxNumRequeues, c.NumThreads, c.runMongoDBRoleBindingInjector)
	c.mgRoleBindingInformer.AddEventHandler(queue.NewObservableHandler(c.mgRoleBindingQueue.GetQueue(), apis.EnableStatusSubresource))
	c.mgRoleBindingLister = c.dbInformerFactory.Authorization().V1alpha1().MongoDBRoleBindings().Lister()
}

func (c *Controller) runMongoDBRoleBindingInjector(key string) error {
	obj, exist, err := c.mgRoleBindingInformer.GetIndexer().GetByKey(key)
	if err != nil {
		glog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exist {
		glog.Warningf("MongoDBRoleBinding %s does not exist anymore", key)

	} else {
		mRoleBinding := obj.(*api.MongoDBRoleBinding).DeepCopy()

		glog.Infof("Sync/Add/Update for MongoDBRoleBinding %s/%s", mRoleBinding.Namespace, mRoleBinding.Name)

		if mRoleBinding.DeletionTimestamp != nil {
			if core_util.HasFinalizer(mRoleBinding.ObjectMeta, apis.Finalizer) {
				go c.runMongoDBRoleBindingFinalizer(mRoleBinding, 1*time.Minute, 10*time.Second)
			}
		} else {
			if !core_util.HasFinalizer(mRoleBinding.ObjectMeta, apis.Finalizer) {
				// Add finalizer
				_, _, err = patchutil.PatchMongoDBRoleBinding(c.dbClient.AuthorizationV1alpha1(), mRoleBinding, func(binding *api.MongoDBRoleBinding) *api.MongoDBRoleBinding {
					binding.ObjectMeta = core_util.AddFinalizer(binding.ObjectMeta, apis.Finalizer)
					return binding
				})
				if err != nil {
					return errors.Wrapf(err, "failed to set MongoDBRoleBinding finalizer for %s/%s", mRoleBinding.Namespace, mRoleBinding.Name)
				}
			}

			dbRBClient, err := database.NewDatabaseRoleBindingForMongodb(c.kubeClient, c.dbClient, mRoleBinding)
			if err != nil {
				return err
			}

			err = c.reconcileMongoDBRoleBinding(dbRBClient, mRoleBinding)
			if err != nil {
				return errors.Wrapf(err, "For MongoDBRoleBinding %s/%s", mRoleBinding.Namespace, mRoleBinding.Name)
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
func (c *Controller) reconcileMongoDBRoleBinding(dbRBClient database.DatabaseRoleBindingInterface, mgRoleBinding *api.MongoDBRoleBinding) error {
	var (
		err   error
		credS *corev1.Secret
	)

	var (
		mgRBName   = mgRoleBinding.Name
		ns         = mgRoleBinding.Namespace
		secretName = mgRoleBinding.Spec.Store.Secret
		status     = mgRoleBinding.Status
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
			status.Conditions = []api.MongoDBRoleBindingCondition{
				{
					Type:    "Available",
					Status:  corev1.ConditionFalse,
					Reason:  "FailedToGetCredential",
					Message: err.Error(),
				},
			}

			err2 := c.updateMongoDBRoleBindingStatus(&status, mgRoleBinding)
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

			status.Conditions = []api.MongoDBRoleBindingCondition{
				{
					Type:    "Available",
					Status:  corev1.ConditionFalse,
					Reason:  "FailedToCreateSecret",
					Message: err.Error(),
				},
			}

			err2 = c.updateMongoDBRoleBindingStatus(&status, mgRoleBinding)
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

	err = dbRBClient.CreateRole(getMongoDBRoleName(mgRBName), ns, secretName)
	if err != nil {
		status.Conditions = []api.MongoDBRoleBindingCondition{
			{
				Type:    "Available",
				Status:  corev1.ConditionFalse,
				Reason:  "FailedToCreateRole",
				Message: err.Error(),
			},
		}

		err2 := c.updateMongoDBRoleBindingStatus(&status, mgRoleBinding)
		if err2 != nil {
			return errors.Wrapf(err2, "failed to update status")
		}
		return errors.WithStack(err)
	}

	err = dbRBClient.CreateRoleBinding(getMongoDBRoleBindingName(mgRBName), ns, getMongoDBRoleName(mgRBName), mgRoleBinding.Spec.Subjects)
	if err != nil {
		status.Conditions = []api.MongoDBRoleBindingCondition{
			{
				Type:    "Available",
				Status:  corev1.ConditionFalse,
				Reason:  "FailedToCreateRoleBinding",
				Message: err.Error(),
			},
		}

		err2 := c.updateMongoDBRoleBindingStatus(&status, mgRoleBinding)
		if err2 != nil {
			return errors.Wrapf(err2, "failed to update status")
		}
		return errors.WithStack(err)
	}

	status.Conditions = []api.MongoDBRoleBindingCondition{}
	status.ObservedGeneration = types.NewIntHash(mgRoleBinding.Generation, meta_util.GenerationHash(mgRoleBinding))

	err = c.updateMongoDBRoleBindingStatus(&status, mgRoleBinding)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (c *Controller) updateMongoDBRoleBindingStatus(status *api.MongoDBRoleBindingStatus, mRoleBinding *api.MongoDBRoleBinding) error {
	_, err := patchutil.UpdateMongoDBRoleBindingStatus(c.dbClient.AuthorizationV1alpha1(), mRoleBinding, func(s *api.MongoDBRoleBindingStatus) *api.MongoDBRoleBindingStatus {
		s = status
		return s
	})
	if err != nil {
		return err
	}

	return nil
}

func (c *Controller) runMongoDBRoleBindingFinalizer(mRoleBinding *api.MongoDBRoleBinding, timeout time.Duration, interval time.Duration) {
	id := getMongoDBRoleBindingId(mRoleBinding)

	if _, ok := c.processingFinalizer[id]; ok {
		// already processing
		return
	}

	c.processingFinalizer[id] = true

	stopCh := time.After(timeout)
	finalizationDone := false

	for {
		m, err := c.dbClient.AuthorizationV1alpha1().MongoDBRoleBindings(mRoleBinding.Namespace).Get(mRoleBinding.Name, metav1.GetOptions{})
		if kerr.IsNotFound(err) {
			delete(c.processingFinalizer, id)
			return
		} else if err != nil {
			glog.Errorf("MongoDBRoleBinding %s/%s finalizer: %v", mRoleBinding.Namespace, mRoleBinding.Name, err)
		}

		// to make sure m is not nil
		if m == nil {
			m = mRoleBinding
		}

		select {
		case <-stopCh:
			err := c.removeMongoDBRoleBindingFinalizer(m)
			if err != nil {
				glog.Errorf("MongoDBRoleBinding %s/%s finalizer: %v", m.Namespace, m.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		default:
		}

		if !finalizationDone {
			d, err := database.NewDatabaseRoleBindingForMongodb(c.kubeClient, c.dbClient, m)
			if err != nil {
				glog.Errorf("MongoDBRoleBinding %s/%s finalizer: %v", m.Namespace, m.Name, err)
			} else {
				err = c.finalizeMongoDBRoleBinding(d, m.Status.Lease.ID)
				if err != nil {
					glog.Errorf("MongoDBRoleBinding %s/%s finalizer: %v", m.Namespace, m.Name, err)
				} else {
					finalizationDone = true
				}
			}
		}

		if finalizationDone {
			err := c.removeMongoDBRoleBindingFinalizer(m)
			if err != nil {
				glog.Errorf("MongoDBRoleBinding %s/%s finalizer: %v", m.Namespace, m.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		}

		select {
		case <-stopCh:
			err := c.removeMongoDBRoleBindingFinalizer(m)
			if err != nil {
				glog.Errorf("MongoDBRoleBinding %s/%s finalizer: %v", m.Namespace, m.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		case <-time.After(interval):
		}
	}
}

func (c *Controller) finalizeMongoDBRoleBinding(dbRBClient database.DatabaseRoleBindingInterface, leaseID string) error {
	if leaseID == "" {
		return nil
	}

	err := dbRBClient.RevokeLease(leaseID)
	if err != nil {
		return err
	}
	return nil
}

func (c *Controller) removeMongoDBRoleBindingFinalizer(mRoleBinding *api.MongoDBRoleBinding) error {
	_, _, err := patchutil.PatchMongoDBRoleBinding(c.dbClient.AuthorizationV1alpha1(), mRoleBinding, func(r *api.MongoDBRoleBinding) *api.MongoDBRoleBinding {
		r.ObjectMeta = core_util.RemoveFinalizer(r.ObjectMeta, apis.Finalizer)
		return r
	})
	if err != nil {
		return err
	}
	return nil
}

func getMongoDBRoleBindingId(mRoleBinding *api.MongoDBRoleBinding) string {
	return fmt.Sprintf("%s/%s/%s", api.ResourceMongoDBRoleBinding, mRoleBinding.Namespace, mRoleBinding.Name)
}

func getMongoDBRoleName(name string) string {
	return fmt.Sprintf("mongodbrolebinding-%s-credential-reader", name)
}

func getMongoDBRoleBindingName(name string) string {
	return fmt.Sprintf("mongodbrolebinding-%s-credential-reader", name)
}
