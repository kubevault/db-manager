package database

import (
	"strconv"

	patchutil "github.com/appscode/kutil/rbac/v1"
	vaultapi "github.com/hashicorp/vault/api"
	api "github.com/kubedb/user-manager/apis/authorization/v1alpha1"
	crd "github.com/kubedb/user-manager/client/clientset/versioned"
	"github.com/kubedb/user-manager/pkg/vault"
	"github.com/kubedb/user-manager/pkg/vault/database/mysql"
	"github.com/kubedb/user-manager/pkg/vault/database/postgres"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type DatabaseRoleBinding struct {
	CredentialGetter
	kubeClient  kubernetes.Interface
	vaultClient *vaultapi.Client
	path        string
}

func NewDatabaseRoleBindingForPostgres(k kubernetes.Interface, cr crd.Interface, roleBinding *api.PostgresRoleBinding) (DatabaseRoleBindingInterface, error) {
	pgRole, err := cr.AuthorizationV1alpha1().PostgresRoles(roleBinding.Namespace).Get(roleBinding.Spec.RoleRef, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get postgres role(%s/%s)", roleBinding.Namespace, roleBinding.Spec.RoleRef)
	}

	v, err := vault.NewClient(k, roleBinding.Namespace, pgRole.Spec.Provider.Vault)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create vault client from postgres role(%s/%s) spec.provider.vault", roleBinding.Namespace, roleBinding.Spec.RoleRef)
	}

	p := postgres.NewPostgresRoleBinding(k, v, roleBinding, "database")

	return &DatabaseRoleBinding{
		CredentialGetter: p,
		kubeClient:       k,
		vaultClient:      v,
		path:             "database",
	}, nil
}

func NewDatabaseRoleBindingForMysql(k kubernetes.Interface, cr crd.Interface, roleBinding *api.MysqlRoleBinding) (DatabaseRoleBindingInterface, error) {
	mRole, err := cr.AuthorizationV1alpha1().MysqlRoles(roleBinding.Namespace).Get(roleBinding.Spec.RoleRef, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get mysql role(%s/%s)", roleBinding.Namespace, roleBinding.Spec.RoleRef)
	}

	v, err := vault.NewClient(k, roleBinding.Namespace, mRole.Spec.Provider.Vault)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create vault client from mysql role(%s/%s) spec.provider.vault", roleBinding.Namespace, roleBinding.Spec.RoleRef)
	}

	m := mysql.NewMysqlRoleBinding(k, v, roleBinding, "database")

	return &DatabaseRoleBinding{
		CredentialGetter: m,
		kubeClient:       k,
		vaultClient:      v,
		path:             "database",
	}, nil
}

// Creates a kubernetes secret containing database credential
func (d *DatabaseRoleBinding) CreateSecret(name string, namespace string, cred *vault.DatabaseCredential) error {
	sr := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"lease_id":       []byte(cred.LeaseID),
			"lease_duration": []byte(strconv.FormatInt(cred.LeaseDuration, 10)),
			"renewable":      []byte(strconv.FormatBool(cred.Renewable)),
			"password":       []byte(cred.Data.Password),
			"username":       []byte(cred.Data.Username),
		},
	}

	addOwnerRefToObject(sr, d.AsOwner())

	_, err := d.kubeClient.CoreV1().Secrets(namespace).Create(sr)
	if err != nil {
		return errors.Wrapf(err, "failed to create secret(%s/%s)", sr.Namespace, sr.Name)
	}

	return nil
}

// Creates kubernetes role
func (d *DatabaseRoleBinding) CreateRole(name string, namespace string, secretName string) error {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{
					"", // represents core api
				},
				Resources: []string{
					"secrets",
				},
				ResourceNames: []string{
					secretName,
				},
				Verbs: []string{
					"get",
				},
			},
		},
	}

	addOwnerRefToObject(role, d.AsOwner())

	_, err := d.kubeClient.RbacV1().Roles(role.Namespace).Create(role)
	if err != nil {
		return errors.Wrapf(err, "failed to create rbac role(%s/%s)", role.Namespace, role.Name)
	}
	return nil
}

// Creates kubernetes role binding
func (d *DatabaseRoleBinding) CreateRoleBinding(name string, namespace string, roleName string, subjects []rbacv1.Subject) error {
	rBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     roleName,
		},
		Subjects: subjects,
	}

	addOwnerRefToObject(rBinding, d.AsOwner())

	_, err := d.kubeClient.RbacV1().RoleBindings(rBinding.Namespace).Create(rBinding)
	if err != nil {
		return errors.Wrapf(err, "failed to create rbac role binding(%s/%s)", rBinding.Namespace, rBinding.Name)
	}
	return nil
}

// Updates subjects of kubernetes role binding
func (d *DatabaseRoleBinding) UpdateRoleBinding(name string, namespace string, subjects []rbacv1.Subject) error {
	obj := metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
	}
	_, _, err := patchutil.CreateOrPatchRoleBinding(d.kubeClient, obj, func(role *rbacv1.RoleBinding) *rbacv1.RoleBinding {
		role.Subjects = subjects
		return role
	})
	if err != nil {
		return errors.Wrapf(err, "failed to update subjects of rbac role binding(%s/%s)", namespace, name)
	}
	return nil
}

func (d *DatabaseRoleBinding) RevokeLease(leaseID string) error {
	err := d.vaultClient.Sys().Revoke(leaseID)
	if err != nil {
		return errors.Wrap(err, "failed to revoke lease")
	}
	return nil
}

// addOwnerRefToObject appends the desired OwnerReference to the object
func addOwnerRefToObject(o metav1.Object, r metav1.OwnerReference) {
	o.SetOwnerReferences(append(o.GetOwnerReferences(), r))
}
