package mongodb

import (
	"fmt"

	vaultapi "github.com/hashicorp/vault/api"
	api "github.com/kubedb/user-manager/apis/authorization/v1alpha1"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type MongoDBRole struct {
	mdbRole      *api.MongoDBRole
	vaultClient  *vaultapi.Client
	kubeClient   kubernetes.Interface
	databasePath string
}

func NewMongoDBRole(k kubernetes.Interface, v *vaultapi.Client, mdbRole *api.MongoDBRole, databasePath string) *MongoDBRole {
	return &MongoDBRole{
		mdbRole:      mdbRole,
		vaultClient:  v,
		kubeClient:   k,
		databasePath: databasePath,
	}
}

// https://www.vaultproject.io/api/secret/databases/index.html#configure-connection
// https://www.vaultproject.io/api/secret/databases/mongodb.html#configure-connection
//
// CreateConfig creates database configuration
func (m *MongoDBRole) CreateConfig() error {
	if m.mdbRole.Spec.Database == nil {
		return errors.New("spec.database is not provided")
	}

	cfg := m.mdbRole.Spec.Database
	ns := m.mdbRole.Namespace

	path := fmt.Sprintf("/v1/%s/config/%s", m.databasePath, cfg.Name)
	req := m.vaultClient.NewRequest("POST", path)

	payload := map[string]interface{}{
		"plugin_name":    "mongodb-database-plugin",
		"allowed_roles":  cfg.AllowedRoles,
		"connection_url": cfg.ConnectionUrl,
	}

	if cfg.PluginName != "" {
		payload["plugin_name"] = cfg.PluginName
	}

	if cfg.WriteConcern != "" {
		payload["write_concern"] = cfg.WriteConcern
	}

	sr, err := m.kubeClient.CoreV1().Secrets(ns).Get(cfg.CredentialSecret, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get credential information from secret %s/%s", ns, cfg.CredentialSecret)
	}

	payload["username"] = string(sr.Data["username"])
	payload["password"] = string(sr.Data["password"])

	err = req.SetJSONBody(payload)
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = m.vaultClient.RawRequest(req)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// https://www.vaultproject.io/api/secret/databases/index.html#create-role
//
// CreateRole creates role
func (m *MongoDBRole) CreateRole() error {
	name := m.mdbRole.Name
	pg := m.mdbRole.Spec

	path := fmt.Sprintf("/v1/%s/roles/%s", m.databasePath, name)
	req := m.vaultClient.NewRequest("POST", path)

	payload := map[string]interface{}{
		"db_name":             pg.DBName,
		"creation_statements": pg.CreationStatements,
	}

	if len(pg.RevocationStatements) > 0 {
		payload["revocation_statements"] = pg.RevocationStatements
	}
	if pg.DefaultTTL != "" {
		payload["default_ttl"] = pg.DefaultTTL
	}
	if pg.MaxTTL != "" {
		payload["max_ttl"] = pg.MaxTTL
	}

	err := req.SetJSONBody(payload)
	if err != nil {
		return errors.WithStack(err)
	}

	_, err = m.vaultClient.RawRequest(req)
	if err != nil {
		return errors.Wrapf(err, "failed to create database role %s for config %s", name, pg.DBName)
	}

	return nil
}
