package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/appscode/pat"
	api "github.com/kubedb/apimachinery/apis/authorization/v1alpha1"
	dbfake "github.com/kubedb/apimachinery/client/clientset/versioned/fake"
	dbinformers "github.com/kubedb/apimachinery/client/informers/externalversions"
	"github.com/kubedb/user-manager/pkg/vault/database"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kfake "k8s.io/client-go/kubernetes/fake"
)

type fakeDRole struct {
	errorOccurredInEnableDatabase bool
	errorOccurredInCreateConfig   bool
	errorOccurredInCreateRole     bool
}

func (f *fakeDRole) EnableDatabase() error {
	if f.errorOccurredInEnableDatabase {
		return fmt.Errorf("error")
	}
	return nil
}

func (f *fakeDRole) IsDatabaseEnabled() (bool, error) {
	return true, nil
}

func (f *fakeDRole) DeleteRole(name string) error {
	return nil
}

func (f *fakeDRole) CreateConfig() error {
	if f.errorOccurredInCreateConfig {
		return fmt.Errorf("error")
	}
	return nil
}

func (f *fakeDRole) CreateRole() error {
	if f.errorOccurredInCreateRole {
		return fmt.Errorf("error")
	}
	return nil
}

func setupVaultServer() *httptest.Server {
	m := pat.New()

	m.Del("/v1/database/roles/pg-read", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	m.Del("/v1/database/roles/error", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("error"))
	}))

	return httptest.NewServer(m)
}

func TestUserManagerController_reconcilePostgresRole(t *testing.T) {
	pRole := api.PostgresRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "pg-role",
			Namespace:  "pg",
			Generation: 0,
		},
		Spec: api.PostgresRoleSpec{
			Database: &api.DatabaseConfigForPostgres{
				Name: "test",
			},
			Provider: &api.ProviderSpec{
				Vault: &api.VaultSpec{},
			},
			DBName: "test",
		},
	}

	testData := []struct {
		testName           string
		pRole              api.PostgresRole
		dbRClient          database.DatabaseRoleInterface
		hasStatusCondition bool
		expectedErr        bool
	}{
		{
			testName:           "initial stage, no error",
			pRole:              pRole,
			dbRClient:          &fakeDRole{},
			expectedErr:        false,
			hasStatusCondition: false,
		},
		{
			testName: "initial stage, failed to enable database",
			pRole:    pRole,
			dbRClient: &fakeDRole{
				errorOccurredInEnableDatabase: true,
			},
			expectedErr:        true,
			hasStatusCondition: true,
		},
		{
			testName: "initial stage, failed to create database connection config",
			pRole:    pRole,
			dbRClient: &fakeDRole{
				errorOccurredInCreateConfig: true,
			},
			expectedErr:        true,
			hasStatusCondition: true,
		},
		{
			testName: "initial stage, failed to create database role",
			pRole:    pRole,
			dbRClient: &fakeDRole{
				errorOccurredInCreateRole: true,
			},
			expectedErr:        true,
			hasStatusCondition: true,
		},
		{
			testName:           "update role, successfully updated database role",
			pRole:              func(p api.PostgresRole) api.PostgresRole { p.Generation = 2; p.Status.ObservedGeneration = 1; return p }(pRole),
			dbRClient:          &fakeDRole{},
			expectedErr:        false,
			hasStatusCondition: false,
		},
		{
			testName: "update role, failed to update database role",
			pRole:    func(p api.PostgresRole) api.PostgresRole { p.Generation = 2; p.Status.ObservedGeneration = 1; return p }(pRole),
			dbRClient: &fakeDRole{
				errorOccurredInCreateRole: true,
			},
			expectedErr:        true,
			hasStatusCondition: true,
		},
	}

	for _, test := range testData {
		t.Run(test.testName, func(t *testing.T) {
			c := &Controller{
				kubeClient: kfake.NewSimpleClientset(),
				dbClient:   dbfake.NewSimpleClientset(),
			}
			c.dbInformerFactory = dbinformers.NewSharedInformerFactory(c.dbClient, time.Minute*10)
			c.pgRoleBindingLister = c.dbInformerFactory.Authorization().V1alpha1().PostgresRoleBindings().Lister()

			_, err := c.dbClient.AuthorizationV1alpha1().PostgresRoles(test.pRole.Namespace).Create(&test.pRole)
			if !assert.Nil(t, err) {
				return
			}

			err = c.reconcilePostgresRole(test.dbRClient, &test.pRole)
			if test.expectedErr {
				if assert.NotNil(t, err) {
					if test.hasStatusCondition {
						p, err2 := c.dbClient.AuthorizationV1alpha1().PostgresRoles(test.pRole.Namespace).Get(test.pRole.Name, metav1.GetOptions{})
						if assert.Nil(t, err2) {
							assert.Condition(t, func() (success bool) {
								if len(p.Status.Conditions) == 0 {
									return false
								}
								return true
							}, "should have status.conditions")
						}
					}
				}
			} else {
				if assert.Nil(t, err) {
					p, err2 := c.dbClient.AuthorizationV1alpha1().PostgresRoles(test.pRole.Namespace).Get(test.pRole.Name, metav1.GetOptions{})
					if assert.Nil(t, err2) {
						assert.Condition(t, func() (success bool) {
							if len(p.Status.Conditions) != 0 {
								return false
							}
							return true
						}, "should not have status.conditions")
					}
				}
			}
		})
	}

}

func TestUserManagerController_runPostgresFinalizer(t *testing.T) {
	srv := setupVaultServer()
	defer srv.Close()

	vaultCredentialSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "vault-cred",
		},
		Data: map[string][]byte{
			"token": []byte("root"),
		},
	}

	userManager := &Controller{
		processingFinalizer: map[string]bool{},
		dbClient:            dbfake.NewSimpleClientset(),
		kubeClient:          kfake.NewSimpleClientset(),
	}
	userManager.dbInformerFactory = dbinformers.NewSharedInformerFactory(userManager.dbClient, time.Minute*10)
	userManager.pgRoleBindingLister = userManager.dbInformerFactory.Authorization().V1alpha1().PostgresRoleBindings().Lister()

	provider := &api.ProviderSpec{
		Vault: &api.VaultSpec{
			Address:             srv.URL,
			SkipTLSVerification: true,
			TokenSecret:         vaultCredentialSecret.Name,
		},
	}

	testData := []struct {
		testName            string
		userManger          *Controller
		pgRole              *api.PostgresRole
		createVaultCred     bool
		timeout             time.Duration
		interval            time.Duration
		finishBeforeTimeout bool
	}{
		{
			testName:   "remove finalizer",
			userManger: userManager,
			pgRole: &api.PostgresRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pg-read",
					Finalizers: []string{
						PostgresRoleFinalizer,
					},
				},
				Spec: api.PostgresRoleSpec{
					Provider: provider,
				},
			},
			createVaultCred:     true,
			timeout:             3 * time.Second,
			interval:            1 * time.Second,
			finishBeforeTimeout: true,
		},
		{
			testName:   "run until timeout, remove finalizer",
			userManger: userManager,
			pgRole: &api.PostgresRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pg-read",
					Finalizers: []string{
						PostgresRoleFinalizer,
					},
				},
				Spec: api.PostgresRoleSpec{
					Provider: provider,
				},
			},
			createVaultCred:     false,
			timeout:             3 * time.Second,
			interval:            1 * time.Second,
			finishBeforeTimeout: false,
		},
	}

	for pos, test := range testData {
		t.Run(test.testName, func(t *testing.T) {
			namespace := "pg" + strconv.Itoa(pos)

			vaultCredentialSecret.Namespace = namespace
			test.pgRole.Namespace = namespace

			if test.createVaultCred {
				_, err := test.userManger.kubeClient.CoreV1().Secrets(namespace).Create(vaultCredentialSecret)
				assert.Nil(t, err)
			}

			_, err := test.userManger.dbClient.AuthorizationV1alpha1().PostgresRoles(namespace).Create(test.pgRole)
			if assert.Nil(t, err) {
				start := time.Now().Unix()
				test.userManger.runPostgresRoleFinalizer(test.pgRole, test.timeout, test.interval)

				if test.finishBeforeTimeout {
					assert.Condition(t, func() (success bool) {
						if (time.Now().Unix() - start) < int64(test.timeout.Seconds()) {
							return true
						}
						return false
					})

					assert.Condition(t, func() (success bool) {
						if _, ok := test.userManger.processingFinalizer[getPostgresRoleId(test.pgRole)]; !ok {
							return true
						}
						return false
					})

				} else {
					assert.Condition(t, func() (success bool) {
						if (time.Now().Unix() - start) < int64(test.timeout.Seconds()) {
							return false
						}
						return true
					})

					assert.Condition(t, func() (success bool) {
						if _, ok := test.userManger.processingFinalizer[getPostgresRoleId(test.pgRole)]; !ok {
							return true
						}
						return false
					})
				}

			}
		})
	}
}
