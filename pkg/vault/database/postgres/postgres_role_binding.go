package postgres

import (
	"encoding/json"
	"fmt"
	"strconv"

	patchutil "github.com/appscode/kutil/rbac/v1"
	vaultapi "github.com/hashicorp/vault/api"
	api "github.com/kubedb/user-manager/apis/authorization/v1alpha1"
	crd "github.com/kubedb/user-manager/client/clientset/versioned"
	"github.com/kubedb/user-manager/pkg/vault"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type PostgresRoleBinding struct {
	pgRoleBinding *api.PostgresRoleBinding
	vaultClient   *vaultapi.Client
	kubeClient    kubernetes.Interface
}

func NewPostgresRoleBinding(k kubernetes.Interface, cr crd.Interface, pgRoleBinding *api.PostgresRoleBinding) (*PostgresRoleBinding, error) {
	pgRole, err := cr.AuthorizationV1alpha1().PostgresRoles(pgRoleBinding.Namespace).Get(pgRoleBinding.Spec.RoleRef, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get postgres role(%s/%s)", pgRoleBinding.Namespace, pgRoleBinding.Spec.RoleRef)
	}

	v, err := vault.NewClient(k, pgRoleBinding.Namespace, pgRole.Spec.Provider.Vault)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create vault client from postgres role(%s/%s) spec.provider.vault", pgRoleBinding.Namespace, pgRoleBinding.Spec.RoleRef)
	}

	return &PostgresRoleBinding{
		pgRoleBinding: pgRoleBinding,
		vaultClient:   v,
		kubeClient:    k,
	}, nil
}

// Gets credential from vault
func (p *PostgresRoleBinding) GetCredentials() (*vault.DatabaseCredentials, error) {
	req := p.vaultClient.NewRequest("GET", fmt.Sprintf("/v1/database/creds/%s", p.pgRoleBinding.Spec.RoleRef))
	resp, err := p.vaultClient.RawRequest(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get postgres credential")
	}

	cred := vault.DatabaseCredentials{}

	err = json.NewDecoder(resp.Body).Decode(&cred)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode json from postgres credential response")
	}
	return &cred, nil
}

// Creates a kubernetes secret containing postgres credential
func (p *PostgresRoleBinding) CreateSecret(cred *vault.DatabaseCredentials) error {

	sr := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.pgRoleBinding.Spec.Store.Secret,
			Namespace: p.pgRoleBinding.Namespace,
		},
		Data: map[string][]byte{
			"lease_id":       []byte(cred.LeaseID),
			"lease_duration": []byte(strconv.FormatInt(cred.LeaseDuration, 10)),
			"renewable":      []byte(strconv.FormatBool(cred.Renewable)),
			"password":       []byte(cred.Data.Password),
			"username":       []byte(cred.Data.Username),
		},
	}

	addOwnerRefToObject(sr, asOwner(p.pgRoleBinding))

	_, err := p.kubeClient.CoreV1().Secrets(p.pgRoleBinding.Namespace).Create(sr)
	if err != nil {
		return errors.Wrapf(err, "failed to create secret(%s/%s)", sr.Namespace, sr.Name)
	}

	return nil
}

// Creates kubernetes role
func (p *PostgresRoleBinding) CreateRole(name, secretName string) error {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: p.pgRoleBinding.Namespace,
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

	addOwnerRefToObject(role, asOwner(p.pgRoleBinding))

	_, err := p.kubeClient.RbacV1().Roles(role.Namespace).Create(role)
	if err != nil {
		return errors.Wrapf(err, "failed to create rbac role(%s/%s)", role.Namespace, role.Name)
	}
	return nil
}

// Creates kubernetes role binding
func (p *PostgresRoleBinding) CreateRoleBinding(name, roleName string) error {
	rBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: p.pgRoleBinding.Namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     roleName,
		},
		Subjects: p.pgRoleBinding.Spec.Subjects,
	}

	addOwnerRefToObject(rBinding, asOwner(p.pgRoleBinding))

	_, err := p.kubeClient.RbacV1().RoleBindings(rBinding.Namespace).Create(rBinding)
	if err != nil {
		return errors.Wrapf(err, "failed to create rbac role binding(%s/%s)", rBinding.Namespace, rBinding.Name)
	}
	return nil
}

// Updates subjects of kubernetes role binding
func (p *PostgresRoleBinding) UpdateRoleBinding(name, namespace string) error {
	obj := metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
	}
	_, _, err := patchutil.CreateOrPatchRoleBinding(p.kubeClient, obj, func(role *rbacv1.RoleBinding) *rbacv1.RoleBinding {
		role.Subjects = p.pgRoleBinding.Spec.Subjects
		return role
	})
	if err != nil {
		return errors.Wrapf(err, "failed to update subjects of rbac role binding(%s/%s)", namespace, name)
	}
	return nil
}

func (p *PostgresRoleBinding) RevokeLease(leaseID string) error {
	err := p.vaultClient.Sys().Revoke(leaseID)
	if err != nil {
		return errors.Wrap(err, "failed to revoke lease")
	}
	return nil
}

// addOwnerRefToObject appends the desired OwnerReference to the object
func addOwnerRefToObject(o metav1.Object, r metav1.OwnerReference) {
	o.SetOwnerReferences(append(o.GetOwnerReferences(), r))
}

// asOwner returns an owner reference
func asOwner(v *api.PostgresRoleBinding) metav1.OwnerReference {
	trueVar := true
	return metav1.OwnerReference{
		APIVersion: api.SchemeGroupVersion.String(),
		Kind:       api.ResourceKindPostgresRoleBinding,
		Name:       v.Name,
		UID:        v.UID,
		Controller: &trueVar,
	}
}
