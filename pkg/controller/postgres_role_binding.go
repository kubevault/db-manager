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
	PostgresRoleBindingFinalizer = "database.postgres.rolebinding"
)

const (
	PhaseSuccess           api.PostgresRoleBindingPhase = "Success"
	PhaseInit              api.PostgresRoleBindingPhase = "Init"
	PhaseGetCredential     api.PostgresRoleBindingPhase = "GetCredential"
	PhaseCreateSecret      api.PostgresRoleBindingPhase = "CreateSecret"
	PhaseCreateRole        api.PostgresRoleBindingPhase = "CreateRole"
	PhaseCreateRoleBinding api.PostgresRoleBindingPhase = "CreateRoleBinding"
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
		glog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exist {
		glog.Warningf("PostgresRoleBinding %s does not exist anymore\n", key)

	} else {
		pgRoleBinding := obj.(*api.PostgresRoleBinding).DeepCopy()

		glog.Infof("Sync/Add/Update for PostgresRoleBinding %s/%s\n", pgRoleBinding.Namespace, pgRoleBinding.Name)

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
				return err
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
	if pgRoleBinding.Status.ObservedGeneration == 0 { // initial stage
		var (
			cred *vault.DatabaseCredential
			err  error
		)

		status := pgRoleBinding.Status
		name := pgRoleBinding.Name
		namespace := pgRoleBinding.Namespace
		roleName := getPostgresRbacRoleName(name)
		roleBindingName := getPostgresRbacRoleBindingName(name)
		storeSecret := pgRoleBinding.Spec.Store.Secret

		if status.Phase == "" || status.Phase == PhaseGetCredential || status.Phase == PhaseCreateSecret {
			status.Phase = PhaseGetCredential

			cred, err = dbRBClient.GetCredential()
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
					return errors.Wrapf(err2, "for postgresRoleBinding(%s/%s): failed to update status", namespace, name)
				}

				return errors.Wrapf(err, "for postgresRoleBinding(%s/%s)", namespace, name)
			}

			glog.Infof("for postgresRoleBinding(%s/%s): getting postgres credential is successful\n", namespace, name)

			// add lease info
			d := time.Duration(cred.LeaseDuration)
			status.Lease = api.LeaseData{
				ID:            cred.LeaseID,
				Duration:      cred.LeaseDuration,
				RenewDeadline: time.Now().Add(time.Second * d).Unix(),
			}

			// next phase
			status.Phase = PhaseCreateSecret
		}

		if status.Phase == PhaseCreateSecret {
			err := dbRBClient.CreateSecret(storeSecret, namespace, cred)
			if err != nil {
				err2 := dbRBClient.RevokeLease(cred.LeaseID)
				if err2 != nil {
					return errors.Wrapf(err2, "for postgresRoleBinding(%s/%s): failed to revoke lease", namespace, name)
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
					return errors.Wrapf(err2, "for postgresRoleBinding(%s/%s): failed to update status", namespace, name)
				}

				return errors.Wrapf(err, "for postgresRoleBinding(%s/%s)", namespace, name)
			}
			glog.Infof("for postgresRoleBinding(%s/%s): creating secret(%s/%s) is successful\n", namespace, name, namespace, pgRoleBinding.Spec.Store.Secret)

			// next phase
			status.Phase = PhaseCreateRole
		}

		if status.Phase == PhaseCreateRole {
			err := dbRBClient.CreateRole(roleName, namespace, storeSecret)
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
					return errors.Wrapf(err2, "for postgresRoleBinding(%s/%s): failed to update status", namespace, name)
				}

				return errors.Wrapf(err, "for postgresRoleBinding(%s/%s)", namespace, name)
			}
			glog.Infof("for postgresRoleBinding(%s/%s): creating rbac role(%s/%s) is successful\n", namespace, name, namespace, roleName)

			//next phase
			status.Phase = PhaseCreateRoleBinding
		}

		if status.Phase == PhaseCreateRoleBinding {
			err := dbRBClient.CreateRoleBinding(roleBindingName, namespace, roleName, pgRoleBinding.Spec.Subjects)
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
					return errors.Wrapf(err2, "for postgresRoleBinding(%s/%s): failed to update status", namespace, name)
				}

				return errors.Wrapf(err, "for postgresRoleBinding(%s/%s)", namespace, name)
			}
			glog.Infof("for postgresRoleBinding(%s/%s): creating rbac role binding(%s/%s) is successful\n", namespace, name, namespace, roleBindingName)
		}

		status.Phase = PhaseSuccess
		status.Conditions = []api.PostgresRoleBindingCondition{}
		status.ObservedGeneration = pgRoleBinding.GetGeneration()

		err = c.updatePostgresRoleBindingStatus(&status, pgRoleBinding)
		if err != nil {
			return errors.Wrapf(err, "for postgresRoleBinding(%s/%s): failed to update status", namespace, name)
		}

	} else {
		// sync role binding
		// - update role binding
		if pgRoleBinding.ObjectMeta.Generation > pgRoleBinding.Status.ObservedGeneration {
			name := pgRoleBinding.Name
			namespace := pgRoleBinding.Namespace
			status := pgRoleBinding.Status

			err := dbRBClient.UpdateRoleBinding(getPostgresRbacRoleBindingName(name), namespace, pgRoleBinding.Spec.Subjects)
			if err != nil {
				status.Conditions = []api.PostgresRoleBindingCondition{
					{
						Type:    "Available",
						Status:  corev1.ConditionFalse,
						Reason:  "FailedToUpdateRoleBinding",
						Message: err.Error(),
					},
				}

				err2 := c.updatePostgresRoleBindingStatus(&status, pgRoleBinding)
				if err2 != nil {
					return errors.Wrapf(err2, "for postgresRoleBinding(%s/%s): failed to update status", namespace, name)
				}

				return errors.Wrapf(err, "for postgresRoleBinding(%s/%s)", namespace, name)
			}

			status.Conditions = []api.PostgresRoleBindingCondition{}
			status.ObservedGeneration = pgRoleBinding.ObjectMeta.Generation

			err = c.updatePostgresRoleBindingStatus(&status, pgRoleBinding)
			if err != nil {
				return errors.Wrapf(err, "for postgresRoleBinding(%s/%s)", namespace, name)
			}
		}
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

func getPostgresRbacRoleName(name string) string {
	return fmt.Sprintf("postgresrolebinding-%s-credential-reader", name)
}

func getPostgresRbacRoleBindingName(name string) string {
	return fmt.Sprintf("postgresrolebinding-%s-credential-reader", name)
}
