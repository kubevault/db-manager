package controller

import (
	kutilcorev1 "github.com/appscode/kutil/core/v1"
	"github.com/appscode/kutil/tools/queue"
	"github.com/golang/glog"
	api "github.com/kubedb/user-manager/apis/authorization/v1alpha1"
	patchutil "github.com/kubedb/user-manager/client/clientset/versioned/typed/authorization/v1alpha1/util"
	"github.com/kubedb/user-manager/pkg/vault"
	"github.com/kubedb/user-manager/pkg/vault/database"
	"github.com/kubedb/user-manager/pkg/vault/database/postgres"
	"github.com/pkg/errors"
)

const (
	PostgresRoleFinalizer = "database.postgres.role"
)

func (c *UserManagerController) initPostgresRoleWatcher() {
	c.postgresRoleInformer = c.dbInformerFactory.Authorization().V1alpha1().PostgresRoles().Informer()
	c.postgresRoleQueue = queue.New(api.ResourceKindPostgresRole, c.MaxNumRequeues, c.NumThreads, c.runPostgresRoleInjector)

	// TODO: add custom event handler?
	c.postgresRoleInformer.AddEventHandler(queue.DefaultEventHandler(c.postgresRoleQueue.GetQueue()))
	c.postgresRoleLister = c.dbInformerFactory.Authorization().V1alpha1().PostgresRoles().Lister()
}

func (c *UserManagerController) runPostgresRoleInjector(key string) error {
	obj, exist, err := c.postgresRoleInformer.GetIndexer().GetByKey(key)
	if err != nil {
		glog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exist {
		glog.Warningf("PostgresRole %s does not exist anymore\n", key)

	} else {
		pgRole := obj.(*api.PostgresRole)

		glog.Infof("Sync/Add/Update for PostgresRole %s/%s\n", pgRole.Namespace, pgRole.Name)

		if pgRole.DeletionTimestamp != nil {
			if kutilcorev1.HasFinalizer(pgRole.ObjectMeta, PostgresRoleFinalizer) {
				vClient, err := vault.NewClient(c.kubeClient, pgRole.Namespace, pgRole.Spec.Provider.Vault)
				if err != nil {
					return errors.Wrap(err, "failed to created vault client")
				}

				pg := postgres.NewPostgresRole(c.kubeClient, vClient, pgRole)

				err = pg.DeleteRole()
				if err != nil {
					return errors.Wrap(err, "failed to database role")
				}

				// remove finalizer
				_, _, err = patchutil.PatchPostgresRole(c.dbClient.AuthorizationV1alpha1(), pgRole, func(role *api.PostgresRole) *api.PostgresRole {
					role.ObjectMeta = kutilcorev1.RemoveFinalizer(role.ObjectMeta, PostgresRoleFinalizer)
					return role
				})
				if err != nil {
					return errors.Wrapf(err, "failed to remove postgresRole finalizer for (%s/%s)", pgRole.Namespace, pgRole.Name)
				}
			}

		} else if !kutilcorev1.HasFinalizer(pgRole.ObjectMeta, PostgresRoleFinalizer) {
			// Add finalizer
			_, _, err := patchutil.PatchPostgresRole(c.dbClient.AuthorizationV1alpha1(), pgRole, func(role *api.PostgresRole) *api.PostgresRole {
				role.ObjectMeta = kutilcorev1.AddFinalizer(role.ObjectMeta, PostgresRoleFinalizer)
				return role
			})
			if err != nil {
				return errors.Wrapf(err, "failed to set postgresRole finalizer for (%s/%s)", pgRole.Namespace, pgRole.Name)
			}
		} else {
			err := c.reconcilePostgresRole(pgRole)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Will do:
//	For vault:
//	  - enable the database secrets engine if it is not already enabled
//	  - configure Vault with the proper postgres plugin and connection information
// 	  - configure a role that maps a name in Vault to an SQL statement to execute to create the database credential.
//    - sync role
func (c *UserManagerController) reconcilePostgresRole(pgRole *api.PostgresRole) error {
	if pgRole.Status.ObservedGeneration == 0 { // initial stage

		vClient, err := vault.NewClient(c.kubeClient, pgRole.Namespace, pgRole.Spec.Provider.Vault)
		if err != nil {
			return errors.Wrap(err, "failed to created vault client")
		}

		// enable the database secrets engine if it is not already enabled
		err = database.EnableDatabase(vClient)
		if err != nil {
			return errors.Wrap(err, "failed to enable database secret engine")
		}

		pg := postgres.NewPostgresRole(c.kubeClient, vClient, pgRole)

		// create database config for postgres
		err = pg.CreateConfig()
		if err != nil {
			return errors.Wrapf(err, "failed to created database connection config(%s)", pgRole.Spec.Database.Name)
		}

		// create role
		err = pg.CreateRole()
		if err != nil {
			return errors.Wrap(err, "failed to create role")
		}

		err = c.updatedStatus(&api.PostgresRoleStatus{
			ObservedGeneration: pgRole.ObjectMeta.Generation,
		}, pgRole)
		if err != nil {
			return errors.Wrap(err, "failed to update postgresRole status")
		}

	} else {
		// sync role
		if pgRole.ObjectMeta.Generation > pgRole.Status.ObservedGeneration {
			vClient, err := vault.NewClient(c.kubeClient, pgRole.Namespace, pgRole.Spec.Provider.Vault)
			if err != nil {
				return errors.Wrap(err, "failed to created vault client")
			}

			pg := postgres.NewPostgresRole(c.kubeClient, vClient, pgRole)

			// In vault create role replaces the old role
			err = pg.CreateRole()
			if err != nil {
				return errors.Wrap(err, "failed to update role")
			}

			err = c.updatedStatus(&api.PostgresRoleStatus{
				ObservedGeneration: pgRole.ObjectMeta.Generation,
			}, pgRole)
			if err != nil {
				return errors.Wrap(err, "failed to update postgresRole status")
			}
		}
	}

	return nil
}

func (c UserManagerController) updatedStatus(status *api.PostgresRoleStatus, pgRole *api.PostgresRole) error {
	_, err := patchutil.UpdatePostgresRoleStatus(c.dbClient.AuthorizationV1alpha1(), pgRole, func(s *api.PostgresRoleStatus) *api.PostgresRoleStatus {
		s = status
		return s
	})
	if err != nil {
		return err
	}

	return nil
}
