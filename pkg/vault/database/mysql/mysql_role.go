package mysql

import (
	"fmt"

	vaultapi "github.com/hashicorp/vault/api"
	api "github.com/kubedb/user-manager/apis/authorization/v1alpha1"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type MysqlRole struct {
	mRole        *api.MysqlRole
	vaultClient  *vaultapi.Client
	kubeClient   kubernetes.Interface
	databasePath string
}

func NewMysqlRole(k kubernetes.Interface, v *vaultapi.Client, mRole *api.MysqlRole, databasePath string) *MysqlRole {
	return &MysqlRole{
		mRole:        mRole,
		vaultClient:  v,
		kubeClient:   k,
		databasePath: databasePath,
	}
}

// https://www.vaultproject.io/api/secret/databases/index.html#configure-connection
// https:https://www.vaultproject.io/api/secret/databases/mysql-maria.html#configure-connection
//
// CreateConfig creates database configuration
func (m *MysqlRole) CreateConfig() error {
	if m.mRole.Spec.Database == nil {
		return errors.New("spec.database is not provided")
	}

	cfg := m.mRole.Spec.Database
	ns := m.mRole.Namespace

	req := m.vaultClient.NewRequest("POST", fmt.Sprintf("/v1/database/config/%s", cfg.Name))
	payload := map[string]interface{}{
		"plugin_name":    "mysql-database-plugin",
		"allowed_roles":  cfg.AllowedRoles,
		"connection_url": cfg.ConnectionUrl,
	}

	if cfg.PluginName != "" {
		payload["plugin_name"] = cfg.PluginName
	}

	sr, err := m.kubeClient.CoreV1().Secrets(ns).Get(cfg.CredentialSecret, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get credential information from secret(%s/%s)", ns, cfg.CredentialSecret)
	}

	payload["username"] = string(sr.Data["username"])
	payload["password"] = string(sr.Data["password"])

	if cfg.MaxOpenConnections > 0 {
		payload["max_open_connections"] = cfg.MaxOpenConnections
	}
	if cfg.MaxIdleConnections > 0 {
		payload["max_idle_connections"] = cfg.MaxIdleConnections
	}
	if cfg.MaxConnectionLifetime != "" {
		payload["max_connection_lifetime"] = cfg.MaxConnectionLifetime
	}

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
func (m *MysqlRole) CreateRole() error {
	name := m.mRole.Name
	pg := m.mRole.Spec

	req := m.vaultClient.NewRequest("POST", fmt.Sprintf("/v1/database/roles/%s", name))

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
		return errors.Wrapf(err, "failed to create database role(%s) for config(%s)", name, pg.DBName)
	}

	return nil
}
