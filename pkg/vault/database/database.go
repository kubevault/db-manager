package database

import (
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/pkg/errors"
)

// EnableDatabase enables database secret engine
// It first checks whether database is enabled or not
func EnableDatabase(client *vaultapi.Client) error {
	enabled, err := IsDatabaseEnabled(client)
	if err != nil {
		return err
	}

	if enabled {
		return nil
	}

	err = client.Sys().Mount("database", &vaultapi.MountInput{
		Type: "database",
	})
	if err != nil {
		return err
	}
	return nil
}

// IsDatabaseEnabled checks whether database is enabled or not
func IsDatabaseEnabled(client *vaultapi.Client) (bool, error) {
	mnt, err := client.Sys().ListMounts()
	if err != nil {
		return false, errors.Wrap(err, "failed to list mounted secrets engines")
	}

	for k := range mnt {
		if k == "database/" {
			return true, nil
		}
	}

	return false, nil
}
