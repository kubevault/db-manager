package controller

import (
	"time"

	"github.com/golang/glog"
	api "github.com/kubedb/user-manager/apis/authorization/v1alpha1"
	"github.com/kubedb/user-manager/pkg/vault"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	renewThreshold = time.Second * 50
)

func (u *UserManagerController) RevokeLease(v *api.VaultSpec, namespace string, leaseID string) error {
	if v == nil {
		return errors.New("vault spec is nil")
	}

	cl, err := vault.NewClient(u.kubeClient, namespace, v)
	if err != nil {
		return errors.Wrap(err, "failed to create vault client")
	}

	err = cl.Sys().Revoke(leaseID)
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (u *UserManagerController) LeaseRenewer(duration time.Duration) {
	for {
		select {
		case <-time.After(duration):
			go u.runLeaseRenewerForPostgres(duration)
			go u.runLeaseRenewerForMysql(duration)
			go u.runLeaseRenewerForMongodb(duration)
		}
	}
}

func (u *UserManagerController) runLeaseRenewerForPostgres(duration time.Duration) {
	pgRBList, err := u.pgRoleBindingLister.List(labels.SelectorFromSet(map[string]string{}))
	if err != nil {
		glog.Errorln("Postgres credential lease renewer: ", err)
	} else {
		for _, p := range pgRBList {
			err = u.RenewLeaseForPostgres(p, duration)
			if err != nil {
				glog.Errorf("Postgres credential lease renewer: for PostgresRoleBinding %s/%s: %v", p.Namespace, p.Name, err)
			}
		}
	}
}

func (u *UserManagerController) RenewLeaseForPostgres(p *api.PostgresRoleBinding, duration time.Duration) error {
	if p.Status.Lease.ID == "" {
		return nil
	}

	remaining := p.Status.Lease.RenewDeadline - time.Now().Unix()
	threshold := duration + renewThreshold

	if remaining > int64(threshold.Seconds()) {
		// has enough time to renew it in next time
		return nil
	}

	pgRole, err := u.dbClient.AuthorizationV1alpha1().PostgresRoles(p.Namespace).Get(p.Spec.RoleRef, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get postgres role %s/%s", p.Namespace, p.Spec.RoleRef)
	}

	v, err := vault.NewClient(u.kubeClient, p.Namespace, pgRole.Spec.Provider.Vault)
	if err != nil {
		return errors.Wrapf(err, "failed to create vault client from postgres role %s/%s spec.provider.vault", p.Namespace, p.Spec.RoleRef)
	}

	_, err = v.Sys().Renew(p.Status.Lease.ID, 0)
	if err != nil {
		return errors.Wrap(err, "failed to renew the lease")
	}

	status := p.Status
	status.Lease.RenewDeadline = time.Now().Unix()

	err = u.updatePostgresRoleBindingStatus(&status, p)
	if err != nil {
		return errors.Wrap(err, "failed to update renew deadline")
	}
	return nil
}

func (u *UserManagerController) runLeaseRenewerForMysql(duration time.Duration) {
	mRBList, err := u.myRoleBindingLister.List(labels.SelectorFromSet(map[string]string{}))
	if err != nil {
		glog.Errorln("Mysql credential lease renewer: ", err)
	} else {
		for _, m := range mRBList {
			err = u.RenewLeaseForMysql(m, duration)
			if err != nil {
				glog.Errorf("Mysql credential lease renewer: for MySQLRoleBinding %s/%s: %v", m.Namespace, m.Name, err)
			}
		}
	}
}

func (u *UserManagerController) RenewLeaseForMysql(m *api.MySQLRoleBinding, duration time.Duration) error {
	if m.Status.Lease.ID == "" {
		return nil
	}

	remaining := m.Status.Lease.RenewDeadline - time.Now().Unix()
	threshold := duration + renewThreshold

	if remaining > int64(threshold.Seconds()) {
		// has enough time to renew it in next time
		return nil
	}

	mRole, err := u.dbClient.AuthorizationV1alpha1().MySQLRoles(m.Namespace).Get(m.Spec.RoleRef, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get mysql role %s/%s", m.Namespace, m.Spec.RoleRef)
	}

	v, err := vault.NewClient(u.kubeClient, m.Namespace, mRole.Spec.Provider.Vault)
	if err != nil {
		return errors.Wrapf(err, "failed to create vault client from mysql role %s/%s spec.provider.vault", m.Namespace, m.Spec.RoleRef)
	}

	_, err = v.Sys().Renew(m.Status.Lease.ID, 0)
	if err != nil {
		return errors.Wrap(err, "failed to renew the lease")
	}

	status := m.Status
	status.Lease.RenewDeadline = time.Now().Unix()

	err = u.updateMySQLRoleBindingStatus(&status, m)
	if err != nil {
		return errors.Wrap(err, "failed to update renew deadline")
	}
	return nil
}

func (u *UserManagerController) runLeaseRenewerForMongodb(duration time.Duration) {
	mRBList, err := u.mgRoleBindingLister.List(labels.SelectorFromSet(map[string]string{}))
	if err != nil {
		glog.Errorln("Mongodb credential lease renewer: ", err)
	} else {
		for _, m := range mRBList {
			err = u.RenewLeaseForMongodb(m, duration)
			if err != nil {
				glog.Errorf("Mongodb credential lease renewer: for MongoDBRoleBinding %s/%s: %v", m.Namespace, m.Name, err)
			}
		}
	}
}

func (u *UserManagerController) RenewLeaseForMongodb(m *api.MongoDBRoleBinding, duration time.Duration) error {
	if m.Status.Lease.ID == "" {
		return nil
	}

	remaining := m.Status.Lease.RenewDeadline - time.Now().Unix()
	threshold := duration + renewThreshold

	if remaining > int64(threshold.Seconds()) {
		// has enough time to renew it in next time
		return nil
	}

	mRole, err := u.dbClient.AuthorizationV1alpha1().MongoDBRoles(m.Namespace).Get(m.Spec.RoleRef, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get mongodb role %s/%s", m.Namespace, m.Spec.RoleRef)
	}

	v, err := vault.NewClient(u.kubeClient, m.Namespace, mRole.Spec.Provider.Vault)
	if err != nil {
		return errors.Wrapf(err, "failed to create vault client from mongodb role %s/%s spec.provider.vault", m.Namespace, m.Spec.RoleRef)
	}

	_, err = v.Sys().Renew(m.Status.Lease.ID, 0)
	if err != nil {
		return errors.Wrap(err, "failed to renew the lease")
	}

	status := m.Status
	status.Lease.RenewDeadline = time.Now().Unix()

	err = u.updateMongoDBRoleBindingStatus(&status, m)
	if err != nil {
		return errors.Wrap(err, "failed to update renew deadline")
	}
	return nil
}
