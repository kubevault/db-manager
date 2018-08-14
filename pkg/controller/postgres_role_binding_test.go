package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/appscode/pat"
	api "github.com/kubedb/user-manager/apis/authorization/v1alpha1"
	dbfake "github.com/kubedb/user-manager/client/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kfake "k8s.io/client-go/kubernetes/fake"
)

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

	userManager := &UserManagerController{
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
		userManger          *UserManagerController
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
