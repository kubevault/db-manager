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
