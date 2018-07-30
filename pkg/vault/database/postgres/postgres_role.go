package postgres

import (
	"fmt"

	vaultapi "github.com/hashicorp/vault/api"
	api "github.com/kubedb/user-manager/apis/authorization/v1alpha1"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type PostgresRole struct {
	pgRole      *api.PostgresRole
	vaultClient *vaultapi.Client
	kubeClient  kubernetes.Interface
}

func NewPostgresRole(k kubernetes.Interface, v *vaultapi.Client, pgRole *api.PostgresRole) *PostgresRole {
	return &PostgresRole{
		pgRole:      pgRole,
		vaultClient: v,
		kubeClient:  k,
	}
}

// https://www.vaultproject.io/api/secret/databases/index.html#configure-connection
// https://www.vaultproject.io/api/secret/databases/postgresql.html#configure-connection
//
// CreateConfig creates database configuration
func (p *PostgresRole) CreateConfig() error {
	cfg := p.pgRole.Spec.Database
	ns := p.pgRole.Namespace

	req := p.vaultClient.NewRequest("POST", fmt.Sprintf("/v1/database/config/%s", cfg.Name))
	payload := map[string]interface{}{
		"plugin_name":    "postgresql-database-plugin",
		"allowed_roles":  cfg.AllowedRoles,
		"connection_url": cfg.ConnectionUrl,
	}

	sr, err := p.kubeClient.CoreV1().Secrets(ns).Get(cfg.CredentialSecret, metav1.GetOptions{})
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

	req.SetJSONBody(payload)
	_, err = p.vaultClient.RawRequest(req)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// https://www.vaultproject.io/api/secret/databases/index.html#create-role
//
// CreateRole creates role
func (p *PostgresRole) CreateRole() error {
	name := p.pgRole.Name
	pg := p.pgRole.Spec

	req := p.vaultClient.NewRequest("POST", fmt.Sprintf("/v1/database/roles/%s", name))

	payload := map[string]interface{}{
		"name":                p,
		"db_name":             pg.DBName,
		"creation_statements": pg.CreationStatements,
	}

	if len(pg.RevocationStatements) > 0 {
		payload["revocation_statements"] = pg.RevocationStatements
	}
	if len(pg.RollbackStatements) > 0 {
		payload["rollback_statements"] = pg.RollbackStatements
	}
	if len(pg.RenewStatements) > 0 {
		payload["renew_statements"] = pg.RenewStatements
	}
	if pg.DefaultTTL != "" {
		payload["default_ttl"] = pg.DefaultTTL
	}
	if pg.MaxTTL != "" {
		payload["max_ttl"] = pg.MaxTTL
	}

	req.SetJSONBody(payload)
	_, err := p.vaultClient.RawRequest(req)
	if err != nil {
		return errors.Wrapf(err, "failed to create database role(%s) for config(%s)", name, pg.DBName)
	}

	return nil
}

// https://www.vaultproject.io/api/secret/databases/index.html#delete-role
//
// DeleteRole deletes role
func (p *PostgresRole) DeleteRole() error {
	req := p.vaultClient.NewRequest("DELETE", fmt.Sprintf("/v1/database/roles/%s", p.pgRole.Name))
	_, err := p.vaultClient.RawRequest(req)
	if err != nil {
		return errors.Wrapf(err, "failed to delete database role(%s) for config(%s)", p.pgRole.Name, p.pgRole.Spec.DBName)
	}
	return nil
}
