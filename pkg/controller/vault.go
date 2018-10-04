package controller

import (
	"time"

	"github.com/golang/glog"
	api "github.com/kubedb/apimachinery/apis/authorization/v1alpha1"
	"github.com/kubedb/user-manager/pkg/vault"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	renewThreshold = time.Second * 50
)

func (c *Controller) RevokeLease(authMgrRef api.AuthManagerRef, leaseID string) error {
	if authMgrRef.Type != api.AuthManagerTypeVault {
		return nil
	}
	if authMgrRef.Namespace == nil {
		return errors.New("missing Vault server namespace")
	}
	if authMgrRef.Name == nil {
		return errors.New("missing Vault server name")
	}

	appBinding, err := c.appBindingLister.AppBindings(*authMgrRef.Namespace).Get(*authMgrRef.Name)
	if err != nil {
		return errors.Wrapf(err, "failed to get Vault %s/%s", *authMgrRef.Namespace, *authMgrRef.Name)
	}

	cl, err := vault.NewClient(c.kubeClient, appBinding)
	if err != nil {
		return errors.Wrap(err, "failed to create vault client")
	}

	err = cl.Sys().Revoke(leaseID)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (c *Controller) LeaseRenewer(duration time.Duration) {
	for {
		select {
		case <-time.After(duration):
			go c.runLeaseRenewerForPostgres(duration)
			go c.runLeaseRenewerForMysql(duration)
			go c.runLeaseRenewerForMongodb(duration)
		}
	}
}

func (c *Controller) runLeaseRenewerForPostgres(duration time.Duration) {
	pgRBList, err := c.pgRoleBindingLister.List(labels.Everything())
	if err != nil {
		glog.Errorln("Postgres credential lease renewer: ", err)
	} else {
		for _, p := range pgRBList {
			err = c.RenewLeaseForPostgres(p, duration)
			if err != nil {
				glog.Errorf("Postgres credential lease renewer: for PostgresRoleBinding %s/%s: %v", p.Namespace, p.Name, err)
			}
		}
	}
}

func (c *Controller) RenewLeaseForPostgres(rb *api.PostgresRoleBinding, duration time.Duration) error {
	if rb.Status.Lease.ID == "" {
		return nil
	}

	remaining := rb.Status.Lease.RenewDeadline - time.Now().Unix()
	threshold := duration + renewThreshold

	if remaining > int64(threshold.Seconds()) {
		// has enough time to renew it in next time
		return nil
	}

	role, err := c.pgRoleLister.PostgresRoles(rb.Namespace).Get(rb.Spec.RoleRef)
	if err != nil {
		return errors.Wrapf(err, "failed to get PostgresRole %s/%s", rb.Namespace, rb.Spec.RoleRef)
	}

	authMgrRef := role.Spec.AuthManagerRef
	if authMgrRef.Type != api.AuthManagerTypeVault {
		return nil
	}
	if authMgrRef.Namespace == nil {
		return errors.Wrapf(err, "missing Vault server namespace for PostgresRole %s/%s", rb.Namespace, rb.Spec.RoleRef)
	}
	if authMgrRef.Name == nil {
		return errors.Wrapf(err, "missing Vault server name for PostgresRole %s/%s", rb.Namespace, rb.Spec.RoleRef)
	}

	appBinding, err := c.appBindingLister.AppBindings(*authMgrRef.Namespace).Get(*authMgrRef.Name)
	if err != nil {
		return errors.Wrapf(err, "failed to get Vault %s/%s", *authMgrRef.Namespace, *authMgrRef.Name)
	}

	v, err := vault.NewClient(c.kubeClient, appBinding)
	if err != nil {
		return errors.Wrapf(err, "failed to create vault client from PostgresRole %s/%s spec.provider.vault", rb.Namespace, rb.Spec.RoleRef)
	}

	_, err = v.Sys().Renew(rb.Status.Lease.ID, 0)
	if err != nil {
		return errors.Wrap(err, "failed to renew the lease")
	}

	status := rb.Status
	status.Lease.RenewDeadline = time.Now().Unix()

	err = c.updatePostgresRoleBindingStatus(&status, rb)
	if err != nil {
		return errors.Wrap(err, "failed to update renew deadline")
	}
	return nil
}

func (c *Controller) runLeaseRenewerForMysql(duration time.Duration) {
	mRBList, err := c.myRoleBindingLister.List(labels.Everything())
	if err != nil {
		glog.Errorln("Mysql credential lease renewer: ", err)
	} else {
		for _, m := range mRBList {
			err = c.RenewLeaseForMysql(m, duration)
			if err != nil {
				glog.Errorf("Mysql credential lease renewer: for MySQLRoleBinding %s/%s: %v", m.Namespace, m.Name, err)
			}
		}
	}
}

func (c *Controller) RenewLeaseForMysql(rb *api.MySQLRoleBinding, duration time.Duration) error {
	if rb.Status.Lease.ID == "" {
		return nil
	}

	remaining := rb.Status.Lease.RenewDeadline - time.Now().Unix()
	threshold := duration + renewThreshold

	if remaining > int64(threshold.Seconds()) {
		// has enough time to renew it in next time
		return nil
	}

	role, err := c.myRoleLister.MySQLRoles(rb.Namespace).Get(rb.Spec.RoleRef)
	if err != nil {
		return errors.Wrapf(err, "failed to get MySQLRole %s/%s", rb.Namespace, rb.Spec.RoleRef)
	}

	authMgrRef := role.Spec.AuthManagerRef
	if authMgrRef.Type != api.AuthManagerTypeVault {
		return nil
	}
	if authMgrRef.Namespace == nil {
		return errors.Wrapf(err, "missing Vault server namespace for MySQLRole %s/%s", rb.Namespace, rb.Spec.RoleRef)
	}
	if authMgrRef.Name == nil {
		return errors.Wrapf(err, "missing Vault server name for MySQLRole %s/%s", rb.Namespace, rb.Spec.RoleRef)
	}

	appBinding, err := c.appBindingLister.AppBindings(*authMgrRef.Namespace).Get(*authMgrRef.Name)
	if err != nil {
		return errors.Wrapf(err, "failed to get Vault %s/%s", *authMgrRef.Namespace, *authMgrRef.Name)
	}

	v, err := vault.NewClient(c.kubeClient, appBinding)
	if err != nil {
		return errors.Wrapf(err, "failed to create vault client from MySQLRole %s/%s spec.provider.vault", rb.Namespace, rb.Spec.RoleRef)
	}

	_, err = v.Sys().Renew(rb.Status.Lease.ID, 0)
	if err != nil {
		return errors.Wrap(err, "failed to renew the lease")
	}

	status := rb.Status
	status.Lease.RenewDeadline = time.Now().Unix()

	err = c.updateMySQLRoleBindingStatus(&status, rb)
	if err != nil {
		return errors.Wrap(err, "failed to update renew deadline")
	}
	return nil
}

func (c *Controller) runLeaseRenewerForMongodb(duration time.Duration) {
	mRBList, err := c.mgRoleBindingLister.List(labels.Everything())
	if err != nil {
		glog.Errorln("Mongodb credential lease renewer: ", err)
	} else {
		for _, m := range mRBList {
			err = c.RenewLeaseForMongodb(m, duration)
			if err != nil {
				glog.Errorf("Mongodb credential lease renewer: for MongoDBRoleBinding %s/%s: %v", m.Namespace, m.Name, err)
			}
		}
	}
}

func (c *Controller) RenewLeaseForMongodb(rb *api.MongoDBRoleBinding, duration time.Duration) error {
	if rb.Status.Lease.ID == "" {
		return nil
	}

	remaining := rb.Status.Lease.RenewDeadline - time.Now().Unix()
	threshold := duration + renewThreshold

	if remaining > int64(threshold.Seconds()) {
		// has enough time to renew it in next time
		return nil
	}

	role, err := c.mgRoleLister.MongoDBRoles(rb.Namespace).Get(rb.Spec.RoleRef)
	if err != nil {
		return errors.Wrapf(err, "failed to get mongodb role %s/%s", rb.Namespace, rb.Spec.RoleRef)
	}

	authMgrRef := role.Spec.AuthManagerRef
	if authMgrRef.Type != api.AuthManagerTypeVault {
		return nil
	}
	if authMgrRef.Namespace == nil {
		return errors.Wrapf(err, "missing Vault server namespace for MongoDBRole %s/%s", rb.Namespace, rb.Spec.RoleRef)
	}
	if authMgrRef.Name == nil {
		return errors.Wrapf(err, "missing Vault server name for MongoDBRole %s/%s", rb.Namespace, rb.Spec.RoleRef)
	}

	appBinding, err := c.appBindingLister.AppBindings(*authMgrRef.Namespace).Get(*authMgrRef.Name)
	if err != nil {
		return errors.Wrapf(err, "failed to get Vault %s/%s", *authMgrRef.Namespace, *authMgrRef.Name)
	}

	v, err := vault.NewClient(c.kubeClient, appBinding)
	if err != nil {
		return errors.Wrapf(err, "failed to create vault client from mongodb role %s/%s spec.provider.vault", rb.Namespace, rb.Spec.RoleRef)
	}

	_, err = v.Sys().Renew(rb.Status.Lease.ID, 0)
	if err != nil {
		return errors.Wrap(err, "failed to renew the lease")
	}

	status := rb.Status
	status.Lease.RenewDeadline = time.Now().Unix()

	err = c.updateMongoDBRoleBindingStatus(&status, rb)
	if err != nil {
		return errors.Wrap(err, "failed to update renew deadline")
	}
	return nil
}
