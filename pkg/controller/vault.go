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

func (u *UserManagerController) LeaseRenewer(duration time.Duration) {
	for {
		select {
		case <-time.After(duration):
			pgRBList, err := u.postgresRoleBindingLister.List(labels.SelectorFromSet(map[string]string{}))
			if err != nil {
				glog.Errorln("Postgres credential lease renewer: ", err)
			} else {
				for _, p := range pgRBList {
					err = u.RenewLease(p, duration)
					if err != nil {
						glog.Errorf("Postgres credential lease renewer: for PostgresRoleBinding(%s/%s): %v", p.Namespace, p.Name, err)
					}
				}
			}

		}
	}
}

func (u *UserManagerController) RenewLease(p *api.PostgresRoleBinding, duration time.Duration) error {
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
		return errors.Wrapf(err, "failed to get postgres role(%s/%s)", p.Namespace, p.Spec.RoleRef)
	}

	v, err := vault.NewClient(u.kubeClient, p.Namespace, pgRole.Spec.Provider.Vault)
	if err != nil {
		return errors.Wrapf(err, "failed to create vault client from postgres role(%s/%s) spec.provider.vault", p.Namespace, p.Spec.RoleRef)
	}

	_, err = v.Sys().Renew(p.Status.Lease.ID, 0)
	if err != nil {
		return errors.Wrap(err, "failed to renew the lease")
	}

	status := p.Status
	status.Lease.RenewDeadline = time.Now().Unix()

	err = u.updatedPostgresRoleBindingStatus(&status, p)
	if err != nil {
		return errors.Wrap(err, "failed to update renew deadline")
	}
	return nil
}
