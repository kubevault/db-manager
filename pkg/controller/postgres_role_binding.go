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
	PostgresRoleBindingFinalizer = "database.postgres.rolebinding"
)

func (c *UserManagerController) initPostgresRoleBindingWatcher() {
	c.pgRoleBindingInformer = c.dbInformerFactory.Authorization().V1alpha1().PostgresRoleBindings().Informer()
	c.pgRoleBindingQueue = queue.New(api.ResourceKindPostgresRoleBinding, c.MaxNumRequeues, c.NumThreads, c.runPostgresRoleBindingInjector)
	c.pgRoleBindingInformer.AddEventHandler(queue.NewEventHandler(c.pgRoleBindingQueue.GetQueue(), func(old interface{}, new interface{}) bool {
		oldObj := old.(*api.PostgresRoleBinding)
		newObj := new.(*api.PostgresRoleBinding)
		return newObj.DeletionTimestamp != nil || !newObj.AlreadyObserved(oldObj)
	}))
	c.pgRoleBindingLister = c.dbInformerFactory.Authorization().V1alpha1().PostgresRoleBindings().Lister()
}

func (c *UserManagerController) runPostgresRoleBindingInjector(key string) error {
	obj, exist, err := c.pgRoleBindingInformer.GetIndexer().GetByKey(key)
	if err != nil {
		glog.Errorf("Fetching object with key(%s) from store failed with %v", key, err)
		return err
	}

	if !exist {
		glog.Warningf("PostgresRoleBinding(%s) does not exist anymore\n", key)

	} else {
		pgRoleBinding := obj.(*api.PostgresRoleBinding).DeepCopy()

		glog.Infof("Sync/Add/Update for PostgresRoleBinding(%s/%s)\n", pgRoleBinding.Namespace, pgRoleBinding.Name)

		if pgRoleBinding.DeletionTimestamp != nil {
			if kutilcorev1.HasFinalizer(pgRoleBinding.ObjectMeta, PostgresRoleBindingFinalizer) {
				go c.runPostgresRoleBindingFinalizer(pgRoleBinding, 1*time.Minute, 10*time.Second)
			}

		} else {
			if !kutilcorev1.HasFinalizer(pgRoleBinding.ObjectMeta, PostgresRoleBindingFinalizer) {
				// Add finalizer
				_, _, err = patchutil.PatchPostgresRoleBinding(c.dbClient.AuthorizationV1alpha1(), pgRoleBinding, func(binding *api.PostgresRoleBinding) *api.PostgresRoleBinding {
					binding.ObjectMeta = kutilcorev1.AddFinalizer(binding.ObjectMeta, PostgresRoleBindingFinalizer)
					return binding
				})
				if err != nil {
					return errors.Wrapf(err, "failed to set postgresRoleBinding finalizer for (%s/%s)", pgRoleBinding.Namespace, pgRoleBinding.Name)
				}

			}

			dbRBClient, err := database.NewDatabaseRoleBindingForPostgres(c.kubeClient, c.dbClient, pgRoleBinding)
			if err != nil {
				return err
			}

			err = c.reconcilePostgresRoleBinding(dbRBClient, pgRoleBinding)
			if err != nil {
				return errors.Wrapf(err, "for PostgresRoleBinding(%s/%s):", pgRoleBinding.Namespace, pgRoleBinding.Name)
			}
		}
	}
	return nil
}

// Will do:
//	For vault:
//	  - get postgres credential
//	  - create secret containing credential
//	  - create rbac role and role binding
//    - sync role binding
func (c *UserManagerController) reconcilePostgresRoleBinding(dbRBClient database.DatabaseRoleBindingInterface, pgRoleBinding *api.PostgresRoleBinding) error {
	var (
		err   error
		credS *corev1.Secret
	)

	var (
		pRBName    = pgRoleBinding.Name
		ns         = pgRoleBinding.Namespace
		secretName = pgRoleBinding.Spec.Store.Secret
		status     = pgRoleBinding.Status
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
			status.Conditions = []api.PostgresRoleBindingCondition{
				{
					Type:    "Available",
					Status:  corev1.ConditionFalse,
					Reason:  "FailedToGetCredential",
					Message: err.Error(),
				},
			}

			err2 := c.updatePostgresRoleBindingStatus(&status, pgRoleBinding)
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

			status.Conditions = []api.PostgresRoleBindingCondition{
				{
					Type:    "Available",
					Status:  corev1.ConditionFalse,
					Reason:  "FailedToCreateSecret",
					Message: err.Error(),
				},
			}

			err2 = c.updatePostgresRoleBindingStatus(&status, pgRoleBinding)
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

	err = dbRBClient.CreateRole(getPostgresRoleName(pRBName), ns, secretName)
	if err != nil {
		status.Conditions = []api.PostgresRoleBindingCondition{
			{
				Type:    "Available",
				Status:  corev1.ConditionFalse,
				Reason:  "FailedToCreateRole",
				Message: err.Error(),
			},
		}

		err2 := c.updatePostgresRoleBindingStatus(&status, pgRoleBinding)
		if err2 != nil {
			return errors.Wrapf(err2, "failed to update status")
		}
		return errors.WithStack(err)
	}

	err = dbRBClient.CreateRoleBinding(getPostgresRoleBindingName(pRBName), ns, getPostgresRoleName(pRBName), pgRoleBinding.Spec.Subjects)
	if err != nil {
		status.Conditions = []api.PostgresRoleBindingCondition{
			{
				Type:    "Available",
				Status:  corev1.ConditionFalse,
				Reason:  "FailedToCreateRoleBinding",
				Message: err.Error(),
			},
		}

		err2 := c.updatePostgresRoleBindingStatus(&status, pgRoleBinding)
		if err2 != nil {
			return errors.Wrapf(err2, "failed to update status")
		}
		return errors.WithStack(err)
	}

	status.Conditions = []api.PostgresRoleBindingCondition{}
	status.ObservedGeneration = pgRoleBinding.Generation

	err = c.updatePostgresRoleBindingStatus(&status, pgRoleBinding)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (c *UserManagerController) updatePostgresRoleBindingStatus(status *api.PostgresRoleBindingStatus, pgRoleBinding *api.PostgresRoleBinding) error {
	_, err := patchutil.UpdatePostgresRoleBindingStatus(c.dbClient.AuthorizationV1alpha1(), pgRoleBinding, func(s *api.PostgresRoleBindingStatus) *api.PostgresRoleBindingStatus {
		s = status
		return s
	})
	if err != nil {
		return err
	}
	return nil
}

func (c *UserManagerController) runPostgresRoleBindingFinalizer(pgRoleBinding *api.PostgresRoleBinding, timeout time.Duration, interval time.Duration) {
	id := getPostgresRoleBindingId(pgRoleBinding)

	if _, ok := c.processingFinalizer[id]; ok {
		// already processing
		return
	}

	c.processingFinalizer[id] = true

	stopCh := time.After(timeout)
	finalizationDone := false

	for {
		p, err := c.dbClient.AuthorizationV1alpha1().PostgresRoleBindings(pgRoleBinding.Namespace).Get(pgRoleBinding.Name, metav1.GetOptions{})
		if kerr.IsNotFound(err) {
			delete(c.processingFinalizer, id)
			return
		} else if err != nil {
			glog.Errorf("PostgresRoleBinding(%s/%s) finalizer: %v\n", pgRoleBinding.Namespace, pgRoleBinding.Name, err)
		}

		// to make sure p is not nil
		if p == nil {
			p = pgRoleBinding
		}

		select {
		case <-stopCh:
			err := c.removePostgresRoleBindingFinalizer(p)
			if err != nil {
				glog.Errorf("PostgresRoleBinding(%s/%s) finalizer: %v\n", p.Namespace, p.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		default:
		}

		if !finalizationDone {
			d, err := database.NewDatabaseRoleBindingForPostgres(c.kubeClient, c.dbClient, p)
			if err != nil {
				glog.Errorf("PostgresRoleBinding(%s/%s) finalizer: %v\n", p.Namespace, p.Name, err)
			} else {
				err = c.finalizePostgresRoleBinding(d, p.Status.Lease.ID)
				if err != nil {
					glog.Errorf("PostgresRoleBinding(%s/%s) finalizer: %v\n", p.Namespace, p.Name, err)
				} else {
					finalizationDone = true
				}
			}
		}

		if finalizationDone {
			err := c.removePostgresRoleBindingFinalizer(p)
			if err != nil {
				glog.Errorf("PostgresRoleBinding(%s/%s) finalizer: %v\n", p.Namespace, p.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		}

		select {
		case <-stopCh:
			err := c.removePostgresRoleBindingFinalizer(p)
			if err != nil {
				glog.Errorf("PostgresRoleBinding(%s/%s) finalizer: %v\n", p.Namespace, p.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		case <-time.After(interval):
		}
	}
}

func (c *UserManagerController) finalizePostgresRoleBinding(dbRBClient database.DatabaseRoleBindingInterface, leaseID string) error {
	if leaseID == "" {
		return nil
	}

	err := dbRBClient.RevokeLease(leaseID)
	if err != nil {
		return err
	}
	return nil
}

func (c *UserManagerController) removePostgresRoleBindingFinalizer(pgRoleBinding *api.PostgresRoleBinding) error {
	_, _, err := patchutil.PatchPostgresRoleBinding(c.dbClient.AuthorizationV1alpha1(), pgRoleBinding, func(r *api.PostgresRoleBinding) *api.PostgresRoleBinding {
		r.ObjectMeta = kutilcorev1.RemoveFinalizer(r.ObjectMeta, PostgresRoleBindingFinalizer)
		return r
	})
	if err != nil {
		return err
	}
	return nil
}

func getPostgresRoleBindingId(pgRoleBinding *api.PostgresRoleBinding) string {
	return fmt.Sprintf("%s/%s/%s", api.ResourcePostgresRoleBinding, pgRoleBinding.Namespace, pgRoleBinding.Name)
}

func getPostgresRoleName(name string) string {
	return fmt.Sprintf("postgresrolebinding-%s-credential-reader", name)
}

func getPostgresRoleBindingName(name string) string {
	return fmt.Sprintf("postgresrolebinding-%s-credential-reader", name)
}
