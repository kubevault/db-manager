package controller

import (
	"fmt"
	"time"

	"github.com/appscode/go/crypto/rand"
	core_util "github.com/appscode/kutil/core/v1"
	"github.com/appscode/kutil/tools/queue"
	"github.com/golang/glog"
	"github.com/kubedb/apimachinery/apis"
	api "github.com/kubedb/apimachinery/apis/authorization/v1alpha1"
	patchutil "github.com/kubedb/apimachinery/client/clientset/versioned/typed/authorization/v1alpha1/util"
	"github.com/kubevault/db-manager/pkg/vault/database"
	"github.com/pkg/errors"
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const RequestFailed api.RequestConditionType = "Failed"

func (c *Controller) initDatabaseAccessWatcher() {
	c.dbAccessInformer = c.dbInformerFactory.Authorization().V1alpha1().DatabaseAccessRequests().Informer()
	c.dbAccessQueue = queue.New(api.ResourceKindDatabaseAccessRequest, c.MaxNumRequeues, c.NumThreads, c.runDatabaseAccessRequestInjector)
	c.dbAccessInformer.AddEventHandler(queue.NewEventHandler(c.dbAccessQueue.GetQueue(), func(oldObj, newObj interface{}) bool {
		old := oldObj.(*api.DatabaseAccessRequest)
		nu := newObj.(*api.DatabaseAccessRequest)

		oldCondType := ""
		nuCondType := ""
		for _, c := range old.Status.Conditions {
			if c.Type == api.AccessApproved || c.Type == api.AccessDenied {
				oldCondType = string(c.Type)
			}
		}
		for _, c := range nu.Status.Conditions {
			if c.Type == api.AccessApproved || c.Type == api.AccessDenied {
				nuCondType = string(c.Type)
			}
		}
		if oldCondType != nuCondType {
			return true
		}
		return nu.GetDeletionTimestamp() != nil
	}))
	c.dbAccessLister = c.dbInformerFactory.Authorization().V1alpha1().DatabaseAccessRequests().Lister()
}

func (c *Controller) runDatabaseAccessRequestInjector(key string) error {
	obj, exist, err := c.dbAccessInformer.GetIndexer().GetByKey(key)
	if err != nil {
		glog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exist {
		glog.Warningf("DatabaseAccessRequest %s does not exist anymore", key)

	} else {
		dbAccessReq := obj.(*api.DatabaseAccessRequest).DeepCopy()

		glog.Infof("Sync/Add/Update for DatabaseAccessRequest %s/%s", dbAccessReq.Namespace, dbAccessReq.Name)

		if dbAccessReq.DeletionTimestamp != nil {
			if core_util.HasFinalizer(dbAccessReq.ObjectMeta, apis.Finalizer) {
				go c.runDatabaseAccessRequestFinalizer(dbAccessReq, finalizerTimeout, finalizerInterval)
			}
		} else {
			if !core_util.HasFinalizer(dbAccessReq.ObjectMeta, apis.Finalizer) {
				// Add finalizer
				_, _, err = patchutil.PatchDatabaseAccessRequest(c.dbClient.AuthorizationV1alpha1(), dbAccessReq, func(binding *api.DatabaseAccessRequest) *api.DatabaseAccessRequest {
					binding.ObjectMeta = core_util.AddFinalizer(binding.ObjectMeta, apis.Finalizer)
					return binding
				})
				if err != nil {
					return errors.Wrapf(err, "failed to set DatabaseAccessRequest finalizer for %s/%s", dbAccessReq.Namespace, dbAccessReq.Name)
				}
			}

			var condType api.RequestConditionType
			for _, c := range dbAccessReq.Status.Conditions {
				if c.Type == api.AccessApproved || c.Type == api.AccessDenied {
					condType = c.Type
				}
			}

			if condType == api.AccessApproved {
				dbCredManager, err := database.NewDatabaseCredentialManager(c.kubeClient, c.catalogClient.AppcatalogV1alpha1(), c.dbClient, dbAccessReq)
				if err != nil {
					return err
				}

				err = c.reconcileDatabaseAccessRequest(dbCredManager, dbAccessReq)
				if err != nil {
					return errors.Wrapf(err, "For DatabaseAccessRequest %s/%s", dbAccessReq.Namespace, dbAccessReq.Name)
				}
			} else if condType == api.AccessDenied {
				glog.Infof("For DatabaseAccessRequest %s/%s: request is denied", dbAccessReq.Namespace, dbAccessReq.Name)
			} else {
				glog.Infof("For DatabaseAccessRequest %s/%s: request is not approved yet", dbAccessReq.Namespace, dbAccessReq.Name)
			}
		}
	}
	return nil
}

// Will do:
//	For vault:
//	  - get db credential
//	  - create secret containing credential
//	  - create rbac role and role binding
//    - sync role binding
func (c *Controller) reconcileDatabaseAccessRequest(dbRBClient database.DatabaseCredentialManager, dbAccessReq *api.DatabaseAccessRequest) error {
	var (
		name   = dbAccessReq.Name
		ns     = dbAccessReq.Namespace
		status = dbAccessReq.Status
	)

	var secretName string
	if dbAccessReq.Status.Secret != nil {
		secretName = dbAccessReq.Status.Secret.Name
	}

	// check whether lease id exists in .status.lease or not
	// if does not exist in .status.lease, then get credential
	if dbAccessReq.Status.Lease == nil {
		// get database credential
		cred, err := dbRBClient.GetCredential()
		if err != nil {
			status.Conditions = UpsertDatabaseAccessCondition(status.Conditions, api.DatabaseAccessRequestCondition{
				Type:    RequestFailed,
				Reason:  "FailedToGetCredential",
				Message: err.Error(),
			})

			err2 := c.updateDatabaseAccessRequestStatus(&status, dbAccessReq)
			if err2 != nil {
				return errors.Wrapf(err2, "failed to update status")
			}
			return errors.WithStack(err)
		}

		secretName = rand.WithUniqSuffix(name)
		err = dbRBClient.CreateSecret(secretName, ns, cred)
		if err != nil {
			err2 := dbRBClient.RevokeLease(cred.LeaseID)
			if err2 != nil {
				return errors.Wrapf(err2, "failed to revoke lease")
			}

			status.Conditions = UpsertDatabaseAccessCondition(status.Conditions, api.DatabaseAccessRequestCondition{
				Type:    RequestFailed,
				Reason:  "FailedToCreateSecret",
				Message: err.Error(),
			})

			err2 = c.updateDatabaseAccessRequestStatus(&status, dbAccessReq)
			if err2 != nil {
				return errors.Wrapf(err2, "failed to update status")
			}
			return errors.WithStack(err)
		}

		// add lease info in status
		status.Lease = &api.Lease{
			ID: cred.LeaseID,
			Duration: metav1.Duration{
				time.Second * time.Duration(cred.LeaseDuration),
			},
			Renewable: cred.Renewable,
		}

		// assign secret name
		status.Secret = &core.LocalObjectReference{
			Name: secretName,
		}
	}

	err := dbRBClient.CreateRole(getSecretAccessRoleName(secretName), ns, secretName)
	if err != nil {
		status.Conditions = UpsertDatabaseAccessCondition(status.Conditions, api.DatabaseAccessRequestCondition{
			Type:    RequestFailed,
			Reason:  "FailedToCreateRole",
			Message: err.Error(),
		})

		err2 := c.updateDatabaseAccessRequestStatus(&status, dbAccessReq)
		if err2 != nil {
			return errors.Wrapf(err2, "failed to update status")
		}
		return errors.WithStack(err)
	}

	err = dbRBClient.CreateRoleBinding(getSecretAccessRoleName(secretName), ns, getSecretAccessRoleName(secretName), dbAccessReq.Spec.Subjects)
	if err != nil {
		status.Conditions = UpsertDatabaseAccessCondition(status.Conditions, api.DatabaseAccessRequestCondition{
			Type:    RequestFailed,
			Reason:  "FailedToCreateRoleBinding",
			Message: err.Error(),
		})

		err2 := c.updateDatabaseAccessRequestStatus(&status, dbAccessReq)
		if err2 != nil {
			return errors.Wrapf(err2, "failed to update status")
		}
		return errors.WithStack(err)
	}

	status.Conditions = DeleteDatabaseAccessCondition(status.Conditions, api.RequestConditionType(RequestFailed))
	err = c.updateDatabaseAccessRequestStatus(&status, dbAccessReq)
	if err != nil {
		return errors.Wrap(err, "failed to update status")
	}
	return nil
}

func (c *Controller) updateDatabaseAccessRequestStatus(status *api.DatabaseAccessRequestStatus, mRoleBinding *api.DatabaseAccessRequest) error {
	_, err := patchutil.UpdateDatabaseAccessRequestStatus(c.dbClient.AuthorizationV1alpha1(), mRoleBinding, func(s *api.DatabaseAccessRequestStatus) *api.DatabaseAccessRequestStatus {
		s = status
		return s
	})
	return err
}

func (c *Controller) runDatabaseAccessRequestFinalizer(mRoleBinding *api.DatabaseAccessRequest, timeout time.Duration, interval time.Duration) {
	id := getDatabaseAccessRequestId(mRoleBinding)

	if _, ok := c.processingFinalizer[id]; ok {
		// already processing
		return
	}

	c.processingFinalizer[id] = true
	glog.Infof("DatabaseAccessRequest %s/%s finalizer: start processing\n", mRoleBinding.Namespace, mRoleBinding.Name)

	stopCh := time.After(timeout)
	finalizationDone := false
	attempt := 0

	for {
		m, err := c.dbClient.AuthorizationV1alpha1().DatabaseAccessRequests(mRoleBinding.Namespace).Get(mRoleBinding.Name, metav1.GetOptions{})
		if kerr.IsNotFound(err) {
			delete(c.processingFinalizer, id)
			return
		} else if err != nil {
			glog.Errorf("DatabaseAccessRequest %s/%s finalizer: %v", mRoleBinding.Namespace, mRoleBinding.Name, err)
		}

		// to make sure m is not nil
		if m == nil {
			m = mRoleBinding
		}

		glog.Infof("DatabaseAccessRequest %s/%s finalizer: attempt %d\n", mRoleBinding.Namespace, mRoleBinding.Name, attempt)

		select {
		case <-stopCh:
			err := c.removeDatabaseAccessRequestFinalizer(m)
			if err != nil {
				glog.Errorf("DatabaseAccessRequest %s/%s finalizer: %v", m.Namespace, m.Name, err)
			} else {
				delete(c.processingFinalizer, id)
				return
			}
		default:
		}

		if !finalizationDone {
			d, err := database.NewDatabaseCredentialManager(c.kubeClient, c.catalogClient.AppcatalogV1alpha1(), c.dbClient, m)
			if err != nil {
				glog.Errorf("DatabaseAccessRequest %s/%s finalizer: %v", m.Namespace, m.Name, err)
			} else {
				err = c.finalizeDatabaseAccessRequest(d, m.Status.Lease)
				if err != nil {
					glog.Errorf("DatabaseAccessRequest %s/%s finalizer: %v", m.Namespace, m.Name, err)
				} else {
					finalizationDone = true
				}
			}
		}

		if finalizationDone {
			err := c.removeDatabaseAccessRequestFinalizer(m)
			if err != nil {
				glog.Errorf("DatabaseAccessRequest %s/%s finalizer: %v", m.Namespace, m.Name, err)
			} else {
				delete(c.processingFinalizer, id)
				return
			}
		}

		select {
		case <-stopCh:
			err := c.removeDatabaseAccessRequestFinalizer(m)
			if err != nil {
				glog.Errorf("DatabaseAccessRequest %s/%s finalizer: %v", m.Namespace, m.Name, err)
			}
			delete(c.processingFinalizer, id)
			return
		case <-time.After(interval):
		}
		attempt++
	}
}

func (c *Controller) finalizeDatabaseAccessRequest(dbRBClient database.DatabaseCredentialManager, lease *api.Lease) error {
	if lease == nil {
		return nil
	}
	if lease.ID == "" {
		return nil
	}

	err := dbRBClient.RevokeLease(lease.ID)
	return err
}

func (c *Controller) removeDatabaseAccessRequestFinalizer(mRoleBinding *api.DatabaseAccessRequest) error {
	_, _, err := patchutil.PatchDatabaseAccessRequest(c.dbClient.AuthorizationV1alpha1(), mRoleBinding, func(r *api.DatabaseAccessRequest) *api.DatabaseAccessRequest {
		r.ObjectMeta = core_util.RemoveFinalizer(r.ObjectMeta, apis.Finalizer)
		return r
	})
	if err != nil {
		return err
	}
	return nil
}

func getDatabaseAccessRequestId(mRoleBinding *api.DatabaseAccessRequest) string {
	return fmt.Sprintf("%s/%s/%s", api.ResourceDatabaseAccessRequest, mRoleBinding.Namespace, mRoleBinding.Name)
}

func getSecretAccessRoleName(name string) string {
	return fmt.Sprintf("%s-credential-reader", name)
}

func UpsertDatabaseAccessCondition(condList []api.DatabaseAccessRequestCondition, cond api.DatabaseAccessRequestCondition) []api.DatabaseAccessRequestCondition {
	res := []api.DatabaseAccessRequestCondition{}
	for _, c := range condList {
		if c.Type == cond.Type {
			res = append(res, cond)
		} else {
			res = append(res, c)
		}
	}

	return res
}

func DeleteDatabaseAccessCondition(condList []api.DatabaseAccessRequestCondition, condType api.RequestConditionType) []api.DatabaseAccessRequestCondition {
	res := []api.DatabaseAccessRequestCondition{}
	for _, c := range condList {
		if c.Type != condType {
			res = append(res, c)
		}
	}
	return res
}
