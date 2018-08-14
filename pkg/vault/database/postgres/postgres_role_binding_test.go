package postgres

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/appscode/pat"
	vaultapi "github.com/hashicorp/vault/api"
	api "github.com/kubedb/user-manager/apis/authorization/v1alpha1"
	"github.com/kubedb/user-manager/pkg/vault"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kfake "k8s.io/client-go/kubernetes/fake"
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
	m.Put("/v1/sys/leases/revoke/success", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}))
	m.Put("/v1/sys/leases/revoke/error", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("error"))
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
				vaultClient: cl,
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
				vaultClient: cl,
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
				vaultClient: cl,
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
			cred, err := test.pgClient.GetCredentials()
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

func TestPostgresRoleBinding_CreateSecret(t *testing.T) {

	pg := &PostgresRoleBinding{
		pgRoleBinding: &api.PostgresRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pg-role-binding",
				Namespace: "pg",
			},
			Spec: api.PostgresRoleBindingSpec{
				Store: api.Store{
					Secret: "pg-cred",
				},
			},
		},
	}

	cred := &vault.DatabaseCredentials{
		LeaseID:       "1204",
		LeaseDuration: 300,
		Data: struct {
			Password string `json:"password"`
			Username string `json:"username"`
		}{
			"1234",
			"nahid",
		},
	}

	testData := []struct {
		testName    string
		pgClient    *PostgresRoleBinding
		cred        *vault.DatabaseCredentials
		expectedErr bool
	}{
		{
			testName:    "Successfully secret created",
			pgClient:    pg,
			cred:        cred,
			expectedErr: false,
		},
		{
			testName:    "Failed to create secret",
			pgClient:    pg,
			cred:        cred,
			expectedErr: true,
		},
	}

	for _, test := range testData {
		t.Run(test.testName, func(t *testing.T) {
			p := test.pgClient
			p.kubeClient = kfake.NewSimpleClientset()

			if test.expectedErr {
				_, err := p.kubeClient.CoreV1().Secrets(p.pgRoleBinding.Namespace).Create(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: p.pgRoleBinding.Namespace,
						Name:      p.pgRoleBinding.Spec.Store.Secret,
					},
					Data: map[string][]byte{
						"test": []byte("hi"),
					},
				})

				assert.Nil(t, err)
			}

			err := p.CreateSecret(test.cred)
			if test.expectedErr {
				assert.NotNil(t, err)
			} else {
				if assert.Nil(t, err) {
					sr, err := p.kubeClient.CoreV1().Secrets(p.pgRoleBinding.Namespace).Get(p.pgRoleBinding.Spec.Store.Secret, metav1.GetOptions{})
					if assert.Nil(t, err) {
						assert.Equal(t, test.cred.LeaseID, string(sr.Data["lease_id"]), "lease_id")
						assert.Equal(t, strconv.FormatInt(test.cred.LeaseDuration, 10), string(sr.Data["lease_duration"]), "lease_duration")
						assert.Equal(t, test.cred.Data.Username, string(sr.Data["username"]), "username")
						assert.Equal(t, test.cred.Data.Password, string(sr.Data["password"]), "password")
					}
				}
			}
		})
	}
}

func TestPostgresRoleBinding_CreateRole(t *testing.T) {

	pg := &PostgresRoleBinding{
		pgRoleBinding: &api.PostgresRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pg-role-binding",
				Namespace: "pg",
			},
		},
	}

	testData := []struct {
		testName    string
		pgClient    *PostgresRoleBinding
		expectedErr bool
		roleName    string
		secretName  string
	}{
		{
			testName:    "Successfully role created",
			pgClient:    pg,
			expectedErr: false,
			roleName:    "pg-role",
			secretName:  "pg-cred",
		},
		{
			testName:    "Failed to create role",
			pgClient:    pg,
			expectedErr: true,
			roleName:    "pg-role",
			secretName:  "pg-cred",
		},
	}

	for _, test := range testData {
		t.Run(test.testName, func(t *testing.T) {
			p := test.pgClient
			p.kubeClient = kfake.NewSimpleClientset()

			if test.expectedErr {
				_, err := p.kubeClient.RbacV1().Roles(p.pgRoleBinding.Namespace).Create(&rbacv1.Role{
					ObjectMeta: metav1.ObjectMeta{
						Name:      test.roleName,
						Namespace: p.pgRoleBinding.Namespace,
					},
				})

				assert.Nil(t, err)
			}

			err := p.CreateRole(test.roleName, test.secretName)
			if test.expectedErr {
				assert.NotNil(t, err)
			} else {
				if assert.Nil(t, err) {
					r, err := p.kubeClient.RbacV1().Roles(p.pgRoleBinding.Namespace).Get(test.roleName, metav1.GetOptions{})
					if assert.Nil(t, err) {
						assert.Equal(t, "", r.Rules[0].APIGroups[0], "api group")
						assert.Equal(t, "secrets", r.Rules[0].Resources[0], "resources")
						assert.Equal(t, test.secretName, r.Rules[0].ResourceNames[0], "resource name")
						assert.Equal(t, "get", r.Rules[0].Verbs[0], "verbs")
					}
				}
			}
		})
	}
}

func TestPostgresRoleBinding_CreateRoleBinding(t *testing.T) {

	pg := &PostgresRoleBinding{
		pgRoleBinding: &api.PostgresRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pg-role-binding",
				Namespace: "pg",
			},
		},
	}

	testData := []struct {
		testName        string
		pgClient        *PostgresRoleBinding
		expectedErr     bool
		roleName        string
		roleBindingName string
	}{
		{
			testName:        "Successfully role binding created",
			pgClient:        pg,
			expectedErr:     false,
			roleName:        "pg-role",
			roleBindingName: "pg-role-binding",
		},
		{
			testName:        "Failed to create role binding",
			pgClient:        pg,
			expectedErr:     true,
			roleName:        "pg-role",
			roleBindingName: "pg-role-binding",
		},
	}

	for _, test := range testData {
		t.Run(test.testName, func(t *testing.T) {
			p := test.pgClient
			p.kubeClient = kfake.NewSimpleClientset()

			if test.expectedErr {
				_, err := p.kubeClient.RbacV1().RoleBindings(p.pgRoleBinding.Namespace).Create(&rbacv1.RoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      test.roleBindingName,
						Namespace: p.pgRoleBinding.Namespace,
					},
				})

				assert.Nil(t, err)
			}

			err := p.CreateRoleBinding(test.roleBindingName, test.roleName)
			if test.expectedErr {
				assert.NotNil(t, err)
			} else {
				if assert.Nil(t, err) {
					r, err := p.kubeClient.RbacV1().RoleBindings(p.pgRoleBinding.Namespace).Get(test.roleBindingName, metav1.GetOptions{})
					if assert.Nil(t, err) {
						assert.Equal(t, test.roleName, r.RoleRef.Name, "role ref role name")
						assert.Equal(t, "Role", r.RoleRef.Kind, "role ref role kind")
						assert.Equal(t, rbacv1.GroupName, r.RoleRef.APIGroup, "role ref role api group")
					}
				}
			}
		})
	}
}

func TestPostgresRoleBinding_RevokeLease(t *testing.T) {
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
		leaseID     string
	}{
		{
			testName: "Lease revoke successful",
			pgClient: &PostgresRoleBinding{
				vaultClient: cl,
			},
			leaseID:     "success",
			expectedErr: false,
		},
		{
			testName: "Lease revoke failed",
			pgClient: &PostgresRoleBinding{
				vaultClient: cl,
			},
			leaseID:     "error",
			expectedErr: true,
		},
	}

	for _, test := range testData {
		t.Run(test.testName, func(t *testing.T) {
			err := test.pgClient.RevokeLease(test.leaseID)
			if test.expectedErr {
				assert.NotNil(t, err, "expected error")
			} else {
				assert.Nil(t, err)
			}
		})
	}
}
