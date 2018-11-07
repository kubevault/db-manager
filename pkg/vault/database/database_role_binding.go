package database

import (
	"encoding/json"
	"strconv"

	patchutilv1 "github.com/appscode/kutil/core/v1"
	patchutil "github.com/appscode/kutil/rbac/v1"
	vaultapi "github.com/hashicorp/vault/api"
	api "github.com/kubedb/apimachinery/apis/authorization/v1alpha1"
	crd "github.com/kubedb/apimachinery/client/clientset/versioned"
	"github.com/kubevault/db-manager/pkg/vault"
	"github.com/kubevault/db-manager/pkg/vault/database/mongodb"
	"github.com/kubevault/db-manager/pkg/vault/database/mysql"
	"github.com/kubevault/db-manager/pkg/vault/database/postgres"
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
		return nil, errors.Wrapf(err, "failed to get postgres role %s/%s", roleBinding.Namespace, roleBinding.Spec.RoleRef)
	}

	v, err := getVaultClient(k, roleBinding.Namespace, pgRole.Spec.Provider)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create vault client from postgres role %s/%s spec.provider.vault", roleBinding.Namespace, roleBinding.Spec.RoleRef)
	}

	path := DefaultDatabasePath
	if pgRole.Spec.Provider.Vault.Path != "" {
		path = pgRole.Spec.Provider.Vault.Path
	}

	p := postgres.NewPostgresRoleBinding(k, v, roleBinding, path)

	return &DatabaseRoleBinding{
		CredentialGetter: p,
		kubeClient:       k,
		vaultClient:      v,
		path:             path,
	}, nil
}

func NewDatabaseRoleBindingForMysql(k kubernetes.Interface, cr crd.Interface, roleBinding *api.MySQLRoleBinding) (DatabaseRoleBindingInterface, error) {
	mRole, err := cr.AuthorizationV1alpha1().MySQLRoles(roleBinding.Namespace).Get(roleBinding.Spec.RoleRef, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get mysql role %s/%s", roleBinding.Namespace, roleBinding.Spec.RoleRef)
	}

	v, err := getVaultClient(k, roleBinding.Namespace, mRole.Spec.Provider)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create vault client from mysql role %s/%s spec.provider.vault", roleBinding.Namespace, roleBinding.Spec.RoleRef)
	}

	path := DefaultDatabasePath
	if mRole.Spec.Provider.Vault.Path != "" {
		path = mRole.Spec.Provider.Vault.Path
	}

	m := mysql.NewMySQLRoleBinding(k, v, roleBinding, path)

	return &DatabaseRoleBinding{
		CredentialGetter: m,
		kubeClient:       k,
		vaultClient:      v,
		path:             path,
	}, nil
}

func NewDatabaseRoleBindingForMongodb(k kubernetes.Interface, cr crd.Interface, roleBinding *api.MongoDBRoleBinding) (DatabaseRoleBindingInterface, error) {
	mRole, err := cr.AuthorizationV1alpha1().MongoDBRoles(roleBinding.Namespace).Get(roleBinding.Spec.RoleRef, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get mongodb role %s/%s", roleBinding.Namespace, roleBinding.Spec.RoleRef)
	}

	v, err := getVaultClient(k, roleBinding.Namespace, mRole.Spec.Provider)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create vault client from mongodb role %s/%s spec.provider.vault", roleBinding.Namespace, roleBinding.Spec.RoleRef)
	}

	path := DefaultDatabasePath
	if mRole.Spec.Provider.Vault.Path != "" {
		path = mRole.Spec.Provider.Vault.Path
	}

	m := mongodb.NewMongoDBRoleBinding(k, v, roleBinding, path)

	return &DatabaseRoleBinding{
		CredentialGetter: m,
		kubeClient:       k,
		vaultClient:      v,
		path:             path,
	}, nil
}

// Creates a kubernetes secret containing database credential
func (d *DatabaseRoleBinding) CreateSecret(name string, namespace string, cred *vault.DatabaseCredential) error {
	data := map[string][]byte{}
	if cred != nil {
		data = map[string][]byte{
			"lease_id":       []byte(cred.LeaseID),
			"lease_duration": []byte(strconv.FormatInt(cred.LeaseDuration, 10)),
			"renewable":      []byte(strconv.FormatBool(cred.Renewable)),
			"password":       []byte(cred.Data.Password),
			"username":       []byte(cred.Data.Username),
		}
	}

	obj := metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
	}

	_, _, err := patchutilv1.CreateOrPatchSecret(d.kubeClient, obj, func(s *corev1.Secret) *corev1.Secret {
		s.Data = data
		addOwnerRefToObject(s, d.AsOwner())
		return s
	})
	if err != nil {
		return errors.Wrapf(err, "failed to create/update secret %s/%s", namespace, name)
	}
	return nil
}

// Creates kubernetes role
func (d *DatabaseRoleBinding) CreateRole(name string, namespace string, secretName string) error {
	obj := metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
	}

	_, _, err := patchutil.CreateOrPatchRole(d.kubeClient, obj, func(role *rbacv1.Role) *rbacv1.Role {
		role.Rules = []rbacv1.PolicyRule{
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
		}

		addOwnerRefToObject(role, d.AsOwner())
		return role
	})
	if err != nil {
		return errors.Wrapf(err, "failed to create rbac role %s/%s", namespace, name)
	}
	return nil
}

// Create kubernetes role binding
func (d *DatabaseRoleBinding) CreateRoleBinding(name string, namespace string, roleName string, subjects []rbacv1.Subject) error {
	obj := metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
	}

	_, _, err := patchutil.CreateOrPatchRoleBinding(d.kubeClient, obj, func(role *rbacv1.RoleBinding) *rbacv1.RoleBinding {
		role.RoleRef = rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     roleName,
		}
		role.Subjects = subjects

		addOwnerRefToObject(role, d.AsOwner())
		return role
	})
	if err != nil {
		return errors.Wrapf(err, "failed to create/update rbac role binding %s/%s", namespace, name)
	}
	return nil
}

// https://www.vaultproject.io/api/system/leases.html#read-lease
//
// Whether or not lease is expired in vault
// In vault, lease is revoked if lease is expired
func (d *DatabaseRoleBinding) IsLeaseExpired(leaseID string) (bool, error) {
	if leaseID == "" {
		return true, nil
	}

	req := d.vaultClient.NewRequest("PUT", "/v1/sys/leases/lookup")
	err := req.SetJSONBody(map[string]string{
		"lease_id": leaseID,
	})
	if err != nil {
		return false, errors.WithStack(err)
	}

	resp, err := d.vaultClient.RawRequest(req)
	if resp == nil && err != nil {
		return false, errors.WithStack(err)
	}

	defer resp.Body.Close()
	errResp := vaultapi.ErrorResponse{}
	err = json.NewDecoder(resp.Body).Decode(&errResp)
	if err != nil {
		return false, errors.WithStack(err)
	}

	if len(errResp.Errors) > 0 {
		return true, nil
	}
	return false, nil
}

// RevokeLease revokes respective lease
// It's safe to call multiple time. It doesn't give
// error even if respective lease_id doesn't exist
// but it will give an error if lease_id is empty
func (d *DatabaseRoleBinding) RevokeLease(leaseID string) error {
	err := d.vaultClient.Sys().Revoke(leaseID)
	if err != nil {
		return errors.Wrap(err, "failed to revoke lease")
	}
	return nil
}

// addOwnerRefToObject appends the desired OwnerReference to the object
func addOwnerRefToObject(o metav1.Object, r metav1.OwnerReference) {
	if !IsOwnerRefAlreadyExists(o, r) {
		o.SetOwnerReferences(append(o.GetOwnerReferences(), r))
	}
}

func IsOwnerRefAlreadyExists(o metav1.Object, r metav1.OwnerReference) bool {
	refs := o.GetOwnerReferences()
	for _, u := range refs {
		if u.Name != r.Name &&
			u.UID == r.UID &&
			u.Kind == r.Kind &&
			u.APIVersion == u.APIVersion {
			return true
		}
	}
	return false
}
