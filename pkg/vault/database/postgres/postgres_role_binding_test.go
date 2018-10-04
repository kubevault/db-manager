package postgres

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/appscode/pat"
	vaultapi "github.com/hashicorp/vault/api"
	api "github.com/kubedb/apimachinery/apis/authorization/v1alpha1"
	"github.com/stretchr/testify/assert"
)

const (
	credResponse = `
{
   "lease_id":"1204",
   "lease_duration":300,
   "data":{
      "username":"nahid",
      "password":"1234"
   }
}
`
)

func vaultServer() *httptest.Server {
	m := pat.New()

	m.Get("/v1/database/creds/geterror", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("error"))
	}))
	m.Get("/v1/database/creds/jsonerror", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("json error"))
	}))
	m.Get("/v1/database/creds/success", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(credResponse))
	}))

	return httptest.NewServer(m)
}

func TestPostgresRoleBinding_GetCredentials(t *testing.T) {
	srv := vaultServer()
	defer srv.Close()

	cfg := vaultapi.DefaultConfig()
	cfg.Address = srv.URL

	cl, err := vaultapi.NewClient(cfg)
	if !assert.Nil(t, err, "failed to create vault client") {
		return
	}

	testData := []struct {
		testName    string
		pgClient    *PostgresRoleBinding
		expectedErr bool
	}{
		{
			testName: "Failed to get credential",
			pgClient: &PostgresRoleBinding{
				vaultClient:  cl,
				databasePath: "database",
				pgRoleBinding: &api.PostgresRoleBinding{
					Spec: api.PostgresRoleBindingSpec{
						RoleRef: "geterror",
					},
				},
			},
			expectedErr: true,
		},
		{
			testName: "Failed to decode json",
			pgClient: &PostgresRoleBinding{
				vaultClient:  cl,
				databasePath: "database",
				pgRoleBinding: &api.PostgresRoleBinding{
					Spec: api.PostgresRoleBindingSpec{
						RoleRef: "jsonerror",
					},
				},
			},
			expectedErr: true,
		},
		{
			testName: "Successfully get the credential",
			pgClient: &PostgresRoleBinding{
				vaultClient:  cl,
				databasePath: "database",
				pgRoleBinding: &api.PostgresRoleBinding{
					Spec: api.PostgresRoleBindingSpec{
						RoleRef: "success",
					},
				},
			},
			expectedErr: false,
		},
	}

	for _, test := range testData {
		t.Run(test.testName, func(t *testing.T) {
			cred, err := test.pgClient.GetCredential()
			if test.expectedErr {
				assert.NotNil(t, err, "expected error")
			} else {
				if assert.Nil(t, err) {
					if assert.NotNil(t, cred, "expected credential to be non nil") {
						assert.Equal(t, "1204", cred.LeaseID)
						assert.Equal(t, int64(300), cred.LeaseDuration)
						assert.Equal(t, "nahid", cred.Data.Username)
						assert.Equal(t, "1234", cred.Data.Password)
					}
				}
			}
		})
	}
}
