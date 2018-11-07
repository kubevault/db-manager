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
	"github.com/kubevault/db-manager/pkg/vault"
	"github.com/kubevault/db-manager/pkg/vault/database"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kfake "k8s.io/client-go/kubernetes/fake"
)

var (
	testCred = vault.DatabaseCredential{
		LeaseID:       "testid",
		LeaseDuration: 100,
		Data: struct {
			Password string `json:"password"`
			Username string `json:"username"`
		}{Password: "1234", Username: "nahid"},
		Renewable: true,
	}
)

type fakeDRB struct {
	errorOccurredInCreateSecret      bool
	errorOccurredInCreateRole        bool
	errorOccurredInCreateRoleBinding bool
	errorOccurredInLease             bool
	errorOccurredInRevokeLease       bool
	errorOccurredInGetCredential     bool
}

func (f *fakeDRB) CreateSecret(name string, namespace string, credential *vault.DatabaseCredential) error {
	if f.errorOccurredInCreateSecret {
		return fmt.Errorf("error")
	}
	return nil
}

func (f *fakeDRB) CreateRole(name string, namespace string, secretName string) error {
	if f.errorOccurredInCreateRole {
		return fmt.Errorf("error")
	}
	return nil
}

func (f *fakeDRB) CreateRoleBinding(name string, namespace string, roleName string, subjects []rbacv1.Subject) error {
	if f.errorOccurredInCreateRoleBinding {
		return fmt.Errorf("error")
	}
	return nil
}

func (f *fakeDRB) IsLeaseExpired(leaseID string) (bool, error) {
	if f.errorOccurredInLease {
		return false, fmt.Errorf("error")
	}
	return false, nil
}

func (f *fakeDRB) RevokeLease(leaseID string) error {
	if f.errorOccurredInRevokeLease {
		return fmt.Errorf("error")
	}
	return nil
}

func (f *fakeDRB) GetCredential() (*vault.DatabaseCredential, error) {
	if f.errorOccurredInGetCredential {
		return nil, fmt.Errorf("error")
	}
	return &testCred, nil
}

func (f *fakeDRB) AsOwner() metav1.OwnerReference {
	return metav1.OwnerReference{}
}

func vaultServer() *httptest.Server {
	m := pat.New()

	m.Put("/v1/sys/leases/revoke/read", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}))

	return httptest.NewServer(m)
}

func TestUserManagerController_runPostgresBindingFinalizer(t *testing.T) {
	srv := vaultServer()
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
		pgRoleBinding       *api.PostgresRoleBinding
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
				},
				Spec: api.PostgresRoleSpec{
					Provider: provider,
				},
			},
			pgRoleBinding: &api.PostgresRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pg-bind",
				},
				Spec: api.PostgresRoleBindingSpec{
					RoleRef: "pg-read",
				},
				Status: api.PostgresRoleBindingStatus{
					Lease: api.LeaseData{
						ID: "read",
					},
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
				},
				Spec: api.PostgresRoleSpec{
					Provider: provider,
				},
			},
			pgRoleBinding: &api.PostgresRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pg-bind",
				},
				Spec: api.PostgresRoleBindingSpec{
					RoleRef: "pg-read",
				},
				Status: api.PostgresRoleBindingStatus{
					Lease: api.LeaseData{
						ID: "read",
					},
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
				_, err := test.userManger.dbClient.AuthorizationV1alpha1().PostgresRoleBindings(namespace).Create(test.pgRoleBinding)
				if assert.Nil(t, err) {
					start := time.Now().Unix()
					test.userManger.runPostgresRoleBindingFinalizer(test.pgRoleBinding, test.timeout, test.interval)

					if test.finishBeforeTimeout {
						assert.Condition(t, func() (success bool) {
							if (time.Now().Unix() - start) < int64(test.timeout.Seconds()) {
								return true
							}
							return false
						})

						assert.Condition(t, func() (success bool) {
							if _, ok := test.userManger.processingFinalizer[getPostgresRoleBindingId(test.pgRoleBinding)]; !ok {
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
							if _, ok := test.userManger.processingFinalizer[getPostgresRoleBindingId(test.pgRoleBinding)]; !ok {
								return true
							}
							return false
						})
					}
				}
			}
		})
	}
}

func TestUserManagerController_reconcilePostgresRoleBinding(t *testing.T) {
	pRBinding := api.PostgresRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pg-role_binding",
			Namespace: "pg",
			UID:       "1234",
		},
		Spec: api.PostgresRoleBindingSpec{
			RoleRef: "test",
			Store: api.Store{
				Secret: "pg-cred",
			},
			Subjects: []rbacv1.Subject{
				{
					Namespace: "pg",
					Name:      "sa",
					Kind:      rbacv1.ServiceAccountKind,
				},
			},
		},
	}

	testData := []struct {
		testName           string
		dbRBClient         database.DatabaseRoleBindingInterface
		pRBinding          api.PostgresRoleBinding
		expectedErr        bool
		hasStatusCondition bool
		createCredSecret   bool
	}{
		{
			testName:           "initial stage, on error",
			pRBinding:          pRBinding,
			dbRBClient:         &fakeDRB{},
			expectedErr:        false,
			hasStatusCondition: false,
		},
		{
			testName:  "initial stage, failed to get credential",
			pRBinding: pRBinding,
			dbRBClient: &fakeDRB{
				errorOccurredInGetCredential: true,
			},
			expectedErr:        true,
			hasStatusCondition: true,
		},
		{
			testName:  "initial stage, failed to create secret",
			pRBinding: pRBinding,
			dbRBClient: &fakeDRB{
				errorOccurredInCreateSecret: true,
			},
			expectedErr:        true,
			hasStatusCondition: true,
		},
		{
			testName:  "initial stage, failed to create rbac role",
			pRBinding: pRBinding,
			dbRBClient: &fakeDRB{
				errorOccurredInCreateRole: true,
			},
			expectedErr:        true,
			hasStatusCondition: true,
		},
		{
			testName:  "initial stage, failed to create rbac role binding",
			pRBinding: pRBinding,
			dbRBClient: &fakeDRB{
				errorOccurredInCreateRoleBinding: true,
			},
			expectedErr:        true,
			hasStatusCondition: true,
		},
		{
			testName:  "error in lease check",
			pRBinding: pRBinding,
			dbRBClient: &fakeDRB{
				errorOccurredInLease: true,
			},
			expectedErr:      true,
			createCredSecret: true,
		},
	}

	for _, test := range testData {
		t.Run(test.testName, func(t *testing.T) {
			c := &Controller{
				kubeClient: kfake.NewSimpleClientset(),
				dbClient:   dbfake.NewSimpleClientset(),
			}

			if test.createCredSecret {
				_, err := c.kubeClient.CoreV1().Secrets(test.pRBinding.Namespace).Create(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      test.pRBinding.Spec.Store.Secret,
						Namespace: test.pRBinding.Namespace,
					},
					Data: map[string][]byte{
						"lease_id": []byte("1234"),
					},
				})
				assert.Nil(t, err)
			}

			_, err := c.dbClient.AuthorizationV1alpha1().PostgresRoleBindings(test.pRBinding.Namespace).Create(&test.pRBinding)
			if !assert.Nil(t, err) {
				return
			}

			err = c.reconcilePostgresRoleBinding(test.dbRBClient, &test.pRBinding)
			if test.expectedErr {
				if assert.NotNil(t, err) {
					if test.hasStatusCondition {
						p, err2 := c.dbClient.AuthorizationV1alpha1().PostgresRoleBindings(test.pRBinding.Namespace).Get(test.pRBinding.Name, metav1.GetOptions{})
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
					p, err2 := c.dbClient.AuthorizationV1alpha1().PostgresRoleBindings(test.pRBinding.Namespace).Get(test.pRBinding.Name, metav1.GetOptions{})
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
