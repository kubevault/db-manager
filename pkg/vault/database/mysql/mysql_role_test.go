package mysql

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/appscode/pat"
	vaultapi "github.com/hashicorp/vault/api"
	api "github.com/kubedb/apimachinery/apis/authorization/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kfake "k8s.io/client-go/kubernetes/fake"
)

func setupVaultServer() *httptest.Server {
	m := pat.New()

	m.Post("/v1/database/config/mysql", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var data interface{}
		err := json.NewDecoder(r.Body).Decode(&data)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		} else {
			m := data.(map[string]interface{})
			if v, ok := m["plugin_name"]; !ok || len(v.(string)) == 0 {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("plugin_name doesn't provided"))
				return
			}
			if v, ok := m["allowed_roles"]; !ok || len(v.(string)) == 0 {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("allowed_roles doesn't provided"))
				return
			}
			if v, ok := m["connection_url"]; !ok || len(v.(string)) == 0 {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("connection_url doesn't provided"))
				return
			}
			if v, ok := m["username"]; !ok || len(v.(string)) == 0 {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("username doesn't provided"))
				return
			}
			if v, ok := m["password"]; !ok || len(v.(string)) == 0 {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("username doesn't provided"))
				return
			}

			w.WriteHeader(http.StatusOK)
		}
	}))
	m.Post("/v1/database/roles/m-read", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var data interface{}
		err := json.NewDecoder(r.Body).Decode(&data)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		} else {
			m := data.(map[string]interface{})
			if v, ok := m["db_name"]; !ok || len(v.(string)) == 0 {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("db_name doesn't provided"))
				return
			}
			w.WriteHeader(http.StatusOK)
		}
	}))
	m.Del("/v1/database/roles/m-read", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	m.Del("/v1/database/roles/error", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("error"))
	}))

	return httptest.NewServer(m)
}

func TestMySQLRole_CreateConfig(t *testing.T) {
	srv := setupVaultServer()
	defer srv.Close()

	cfg := vaultapi.DefaultConfig()
	cfg.Address = srv.URL

	cl, err := vaultapi.NewClient(cfg)
	if !assert.Nil(t, err, "failed to create vault client") {
		return
	}

	mySql := &MySQLRole{
		mRole: &api.MySQLRole{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "m-role",
				Namespace: "m",
			},
			Spec: api.MySQLRoleSpec{
				Database: &api.DatabaseConfigForMysql{
					Name:             "mysql",
					AllowedRoles:     "*",
					ConnectionUrl:    "hi.com",
					CredentialSecret: "m-cred",
				},
			},
		},
		vaultClient:  cl,
		databasePath: "database",
	}

	testData := []struct {
		testName               string
		mClient                *MySQLRole
		createCredentialSecret bool
		expectedErr            bool
	}{
		{
			testName:               "Create Config successful",
			mClient:                mySql,
			createCredentialSecret: true,
			expectedErr:            false,
		},
		{
			testName:               "Create Config failed, secret not found",
			mClient:                mySql,
			createCredentialSecret: false,
			expectedErr:            true,
		},
		{
			testName: "Create Config failed, connection_url not provided",
			mClient: func() *MySQLRole {
				p := &MySQLRole{
					mRole: &api.MySQLRole{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "m-role",
							Namespace: "m",
						},
						Spec: api.MySQLRoleSpec{
							Database: &api.DatabaseConfigForMysql{
								Name:             "mysql",
								AllowedRoles:     "*",
								ConnectionUrl:    "",
								CredentialSecret: "m-cred",
							},
						},
					},
					vaultClient: cl,
				}
				return p
			}(),
			createCredentialSecret: true,
			expectedErr:            true,
		},
	}

	for _, test := range testData {
		t.Run(test.testName, func(t *testing.T) {
			m := test.mClient
			m.kubeClient = kfake.NewSimpleClientset()

			if test.createCredentialSecret {
				_, err := m.kubeClient.CoreV1().Secrets(m.mRole.Namespace).Create(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      m.mRole.Spec.Database.CredentialSecret,
						Namespace: m.mRole.Namespace,
					},
					Data: map[string][]byte{
						"username": []byte("nahid"),
						"password": []byte("root"),
					},
				})

				assert.Nil(t, err)
			}

			err := m.CreateConfig()
			if test.expectedErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestMySQLRole_CreateRole(t *testing.T) {
	srv := setupVaultServer()
	defer srv.Close()

	cfg := vaultapi.DefaultConfig()
	cfg.Address = srv.URL

	cl, err := vaultapi.NewClient(cfg)
	if !assert.Nil(t, err, "failed to create vault client") {
		return
	}

	testData := []struct {
		testName    string
		mClient     *MySQLRole
		expectedErr bool
	}{
		{
			testName: "Create Role successful",
			mClient: &MySQLRole{
				mRole: &api.MySQLRole{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "m-read",
						Namespace: "m",
					},
					Spec: api.MySQLRoleSpec{
						DBName:             "mysql",
						CreationStatements: []string{"create table"},
					},
				},
				vaultClient:  cl,
				databasePath: "database",
			},
			expectedErr: false,
		},
		{
			testName: "Create Role failed",
			mClient: &MySQLRole{
				mRole: &api.MySQLRole{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "m-read",
						Namespace: "m",
					},
					Spec: api.MySQLRoleSpec{
						CreationStatements: []string{"create table"},
					},
				},
				vaultClient:  cl,
				databasePath: "database",
			},
			expectedErr: true,
		},
	}

	for _, test := range testData {
		t.Run(test.testName, func(t *testing.T) {
			m := test.mClient

			err := m.CreateRole()
			if test.expectedErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}
