package database

import (
	"fmt"

	vaultapi "github.com/hashicorp/vault/api"
	api "github.com/kubedb/user-manager/apis/authorization/v1alpha1"
	"github.com/kubedb/user-manager/pkg/vault"
	"github.com/kubedb/user-manager/pkg/vault/database/mongodb"
	"github.com/kubedb/user-manager/pkg/vault/database/mysql"
	"github.com/kubedb/user-manager/pkg/vault/database/postgres"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
)

type DatabaseRole struct {
	RoleInterface
	vaultClient *vaultapi.Client
	path        string
}

func NewDatabaseRoleForPostgres(kClient kubernetes.Interface, role *api.PostgresRole) (DatabaseRoleInterface, error) {
	if role.Spec.Provider == nil {
		return nil, errors.New("spec.provider is not provided")
	}
	if role.Spec.Provider.Vault == nil {
		return nil, errors.New("spec.provider.vault is not provided")
	}

	vClient, err := vault.NewClient(kClient, role.Namespace, role.Spec.Provider.Vault)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create vault client")
	}

	d := &DatabaseRole{
		RoleInterface: postgres.NewPostgresRole(kClient, vClient, role, "database"),
		path:          "database",
		vaultClient:   vClient,
	}
	return d, nil
}

func NewDatabaseRoleForMysql(kClient kubernetes.Interface, role *api.MysqlRole) (DatabaseRoleInterface, error) {
	if role.Spec.Provider == nil {
		return nil, errors.New("spec.provider is not provided")
	}
	if role.Spec.Provider.Vault == nil {
		return nil, errors.New("spec.provider.vault is not provided")
	}

	vClient, err := vault.NewClient(kClient, role.Namespace, role.Spec.Provider.Vault)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create vault client")
	}

	d := &DatabaseRole{
		RoleInterface: mysql.NewMysqlRole(kClient, vClient, role, "database"),
		path:          "database",
		vaultClient:   vClient,
	}
	return d, nil
}

func NewDatabaseRoleForMongodb(kClient kubernetes.Interface, role *api.MongodbRole) (DatabaseRoleInterface, error) {
	if role.Spec.Provider == nil {
		return nil, errors.New("spec.provider is not provided")
	}
	if role.Spec.Provider.Vault == nil {
		return nil, errors.New("spec.provider.vault is not provided")
	}

	vClient, err := vault.NewClient(kClient, role.Namespace, role.Spec.Provider.Vault)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create vault client")
	}

	d := &DatabaseRole{
		RoleInterface: mongodb.NewMongodbRole(kClient, vClient, role, "database"),
		path:          "database",
		vaultClient:   vClient,
	}
	return d, nil
}

// EnableDatabase enables database secret engine
// It first checks whether database is enabled or not
func (d *DatabaseRole) EnableDatabase() error {
	enabled, err := d.IsDatabaseEnabled()
	if err != nil {
		return err
	}

	if enabled {
		return nil
	}

	err = d.vaultClient.Sys().Mount(d.path, &vaultapi.MountInput{
		Type: "database",
	})
	if err != nil {
		return err
	}
	return nil
}

// IsDatabaseEnabled checks whether database is enabled or not
func (d *DatabaseRole) IsDatabaseEnabled() (bool, error) {
	mnt, err := d.vaultClient.Sys().ListMounts()
	if err != nil {
		return false, errors.Wrap(err, "failed to list mounted secrets engines")
	}

	mntPath := d.path + "/"
	for k := range mnt {
		if k == mntPath {
			return true, nil
		}
	}
	return false, nil
}

// https://www.vaultproject.io/api/secret/databases/index.html#delete-role
//
// DeleteRole deletes role
// It's safe to call multiple time. It doesn't give
// error even if respective role doesn't exist
func (d *DatabaseRole) DeleteRole(name string) error {
	path := fmt.Sprintf("/v1/%s/roles/%s", d.path, name)
	req := d.vaultClient.NewRequest("DELETE", path)

	_, err := d.vaultClient.RawRequest(req)
	if err != nil {
		return errors.Wrapf(err, "failed to delete database role(%s)", name)
	}
	return nil
}
