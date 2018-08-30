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
	dbinformers "github.com/kubedb/user-manager/client/informers/externalversions"
	"github.com/kubedb/user-manager/pkg/vault/database"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kfake "k8s.io/client-go/kubernetes/fake"
)

func setupVaultServerForMongodb() *httptest.Server {
	m := pat.New()

	m.Del("/v1/database/roles/m-read", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	m.Del("/v1/database/roles/error", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("error"))
	}))

	return httptest.NewServer(m)
}

func TestUserManagerController_runMongodbFinalizer(t *testing.T) {
	srv := setupVaultServerForMongodb()
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
	userManager.dbInformerFactory = dbinformers.NewSharedInformerFactory(userManager.dbClient, time.Minute*10)
	userManager.mgRoleBindingLister = userManager.dbInformerFactory.Authorization().V1alpha1().MongodbRoleBindings().Lister()

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
		mRole               *api.MongodbRole
		createVaultCred     bool
		timeout             time.Duration
		interval            time.Duration
		finishBeforeTimeout bool
	}{
		{
			testName:   "remove finalizer",
			userManger: userManager,
			mRole: &api.MongodbRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: "m-read",
					Finalizers: []string{
						MysqlRoleFinalizer,
					},
				},
				Spec: api.MongodbRoleSpec{
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
			mRole: &api.MongodbRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: "m-read",
					Finalizers: []string{
						MysqlRoleFinalizer,
					},
				},
				Spec: api.MongodbRoleSpec{
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
			namespace := "m" + strconv.Itoa(pos)

			vaultCredentialSecret.Namespace = namespace
			test.mRole.Namespace = namespace

			if test.createVaultCred {
				_, err := test.userManger.kubeClient.CoreV1().Secrets(namespace).Create(vaultCredentialSecret)
				assert.Nil(t, err)
			}

			_, err := test.userManger.dbClient.AuthorizationV1alpha1().MongodbRoles(namespace).Create(test.mRole)
			if assert.Nil(t, err) {
				start := time.Now().Unix()
				test.userManger.runMongodbRoleFinalizer(test.mRole, test.timeout, test.interval)

				if test.finishBeforeTimeout {
					assert.Condition(t, func() (success bool) {
						if (time.Now().Unix() - start) < int64(test.timeout.Seconds()) {
							return true
						}
						return false
					})

					assert.Condition(t, func() (success bool) {
						if _, ok := test.userManger.processingFinalizer[getMongodbRoleId(test.mRole)]; !ok {
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
						if _, ok := test.userManger.processingFinalizer[getMongodbRoleId(test.mRole)]; !ok {
							return true
						}
						return false
					})
				}

			}
		})
	}
}

func TestUserManagerController_reconcileMongodbRole(t *testing.T) {
	mRole := api.MongodbRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "m-role",
			Namespace:  "m",
			Generation: 0,
		},
		Spec: api.MongodbRoleSpec{
			Database: &api.DatabaseConfigForMongodb{
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
		mRole              api.MongodbRole
		dbRClient          database.DatabaseRoleInterface
		hasStatusCondition bool
		expectedErr        bool
	}{
		{
			testName:           "initial stage, no error",
			mRole:              mRole,
			dbRClient:          &fakeDRole{},
			expectedErr:        false,
			hasStatusCondition: false,
		},
		{
			testName: "initial stage, failed to enable database",
			mRole:    mRole,
			dbRClient: &fakeDRole{
				errorOccurredInEnableDatabase: true,
			},
			expectedErr:        true,
			hasStatusCondition: true,
		},
		{
			testName: "initial stage, failed to create database connection config",
			mRole:    mRole,
			dbRClient: &fakeDRole{
				errorOccurredInCreateConfig: true,
			},
			expectedErr:        true,
			hasStatusCondition: true,
		},
		{
			testName: "initial stage, failed to create database role",
			mRole:    mRole,
			dbRClient: &fakeDRole{
				errorOccurredInCreateRole: true,
			},
			expectedErr:        true,
			hasStatusCondition: true,
		},
		{
			testName:           "update role, successfully updated database role",
			mRole:              func(p api.MongodbRole) api.MongodbRole { p.Generation = 2; p.Status.ObservedGeneration = 1; return p }(mRole),
			dbRClient:          &fakeDRole{},
			expectedErr:        false,
			hasStatusCondition: false,
		},
		{
			testName: "update role, failed to update database role",
			mRole:    func(p api.MongodbRole) api.MongodbRole { p.Generation = 2; p.Status.ObservedGeneration = 1; return p }(mRole),
			dbRClient: &fakeDRole{
				errorOccurredInCreateRole: true,
			},
			expectedErr:        true,
			hasStatusCondition: true,
		},
	}

	for _, test := range testData {
		t.Run(test.testName, func(t *testing.T) {
			c := &UserManagerController{
				kubeClient: kfake.NewSimpleClientset(),
				dbClient:   dbfake.NewSimpleClientset(),
			}
			c.dbInformerFactory = dbinformers.NewSharedInformerFactory(c.dbClient, time.Minute*10)
			c.mgRoleBindingLister = c.dbInformerFactory.Authorization().V1alpha1().MongodbRoleBindings().Lister()

			_, err := c.dbClient.AuthorizationV1alpha1().MongodbRoles(test.mRole.Namespace).Create(&test.mRole)
			if !assert.Nil(t, err) {
				return
			}

			err = c.reconcileMongodbRole(test.dbRClient, &test.mRole)
			if test.expectedErr {
				if assert.NotNil(t, err) {
					if test.hasStatusCondition {
						p, err2 := c.dbClient.AuthorizationV1alpha1().MongodbRoles(test.mRole.Namespace).Get(test.mRole.Name, metav1.GetOptions{})
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
					p, err2 := c.dbClient.AuthorizationV1alpha1().MongodbRoles(test.mRole.Namespace).Get(test.mRole.Name, metav1.GetOptions{})
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
