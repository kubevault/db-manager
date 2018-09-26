package mongodb

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

type MongoDBRoleBinding struct {
	mRoleBinding *api.MongoDBRoleBinding
	vaultClient  *vaultapi.Client
	kubeClient   kubernetes.Interface
	databasePath string
}

func NewMongoDBRoleBinding(k kubernetes.Interface, v *vaultapi.Client, mRoleBinding *api.MongoDBRoleBinding, databasePath string) *MongoDBRoleBinding {
	return &MongoDBRoleBinding{
		mRoleBinding: mRoleBinding,
		vaultClient:  v,
		kubeClient:   k,
		databasePath: databasePath,
	}
}

// Gets credential from vault
func (p *MongoDBRoleBinding) GetCredential() (*vault.DatabaseCredential, error) {
	path := fmt.Sprintf("/v1/%s/creds/%s", p.databasePath, p.mRoleBinding.Spec.RoleRef)
	req := p.vaultClient.NewRequest("GET", path)

	resp, err := p.vaultClient.RawRequest(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get mongodb credential")
	}

	cred := vault.DatabaseCredential{}

	err = json.NewDecoder(resp.Body).Decode(&cred)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode json from mongodb credential response")
	}
	return &cred, nil
}

// asOwner returns an owner reference
func (p *MongoDBRoleBinding) AsOwner() metav1.OwnerReference {
	trueVar := true
	return metav1.OwnerReference{
		APIVersion: api.SchemeGroupVersion.String(),
		Kind:       api.ResourceKindMongoDBRoleBinding,
		Name:       p.mRoleBinding.Name,
		UID:        p.mRoleBinding.UID,
		Controller: &trueVar,
	}
}
