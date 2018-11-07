package postgres

import (
	"encoding/json"
	"fmt"

	vaultapi "github.com/hashicorp/vault/api"
	api "github.com/kubedb/apimachinery/apis/authorization/v1alpha1"
	"github.com/kubevault/db-manager/pkg/vault"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type PostgresRoleBinding struct {
	pgRoleBinding *api.PostgresRoleBinding
	vaultClient   *vaultapi.Client
	kubeClient    kubernetes.Interface
	databasePath  string
}

func NewPostgresRoleBinding(k kubernetes.Interface, v *vaultapi.Client, pgRoleBinding *api.PostgresRoleBinding, databasePath string) *PostgresRoleBinding {
	return &PostgresRoleBinding{
		pgRoleBinding: pgRoleBinding,
		vaultClient:   v,
		kubeClient:    k,
		databasePath:  databasePath,
	}
}

// Gets credential from vault
func (p *PostgresRoleBinding) GetCredential() (*vault.DatabaseCredential, error) {
	path := fmt.Sprintf("/v1/%s/creds/%s", p.databasePath, p.pgRoleBinding.Spec.RoleRef)
	req := p.vaultClient.NewRequest("GET", path)
	resp, err := p.vaultClient.RawRequest(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get postgres credential")
	}

	cred := vault.DatabaseCredential{}

	err = json.NewDecoder(resp.Body).Decode(&cred)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode json from postgres credential response")
	}
	return &cred, nil
}

// asOwner returns an owner reference
func (p *PostgresRoleBinding) AsOwner() metav1.OwnerReference {
	trueVar := true
	return metav1.OwnerReference{
		APIVersion: api.SchemeGroupVersion.String(),
		Kind:       api.ResourceKindPostgresRoleBinding,
		Name:       p.pgRoleBinding.Name,
		UID:        p.pgRoleBinding.UID,
		Controller: &trueVar,
	}
}

func (p *PostgresRoleBinding) GetVaultClient() *vaultapi.Client {
	return p.vaultClient
}
