package mysql

import (
	"encoding/json"
	"fmt"

	vaultapi "github.com/hashicorp/vault/api"
	api "github.com/kubedb/user-manager/apis/authorization/v1alpha1"
	"github.com/kubedb/user-manager/pkg/vault"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type MysqlRoleBinding struct {
	mRoleBinding *api.MysqlRoleBinding
	vaultClient  *vaultapi.Client
	kubeClient   kubernetes.Interface
	databasePath string
}

func NewMysqlRoleBinding(k kubernetes.Interface, v *vaultapi.Client, mRoleBinding *api.MysqlRoleBinding, databasePath string) *MysqlRoleBinding {
	return &MysqlRoleBinding{
		mRoleBinding: mRoleBinding,
		vaultClient:  v,
		kubeClient:   k,
		databasePath: databasePath,
	}
}

// Gets credential from vault
func (p *MysqlRoleBinding) GetCredential() (*vault.DatabaseCredential, error) {
	path := fmt.Sprintf("/v1/%s/creds/%s", p.databasePath, p.mRoleBinding.Spec.RoleRef)
	req := p.vaultClient.NewRequest("GET", path)

	resp, err := p.vaultClient.RawRequest(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get mysql credential")
	}

	cred := vault.DatabaseCredential{}

	err = json.NewDecoder(resp.Body).Decode(&cred)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode json from mysql credential response")
	}
	return &cred, nil
}

// asOwner returns an owner reference
func (p *MysqlRoleBinding) AsOwner() metav1.OwnerReference {
	trueVar := true
	return metav1.OwnerReference{
		APIVersion: api.SchemeGroupVersion.String(),
		Kind:       api.ResourceKindMysqlRoleBinding,
		Name:       p.mRoleBinding.Name,
		UID:        p.mRoleBinding.UID,
		Controller: &trueVar,
	}
}
