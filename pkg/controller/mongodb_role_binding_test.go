package controller

import (
	"strconv"
	"testing"
	"time"

	api "github.com/kubedb/user-manager/apis/authorization/v1alpha1"
	dbfake "github.com/kubedb/user-manager/client/clientset/versioned/fake"
	"github.com/kubedb/user-manager/pkg/vault/database"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kfake "k8s.io/client-go/kubernetes/fake"
)

func TestUserManagerController_runMongodbBindingFinalizer(t *testing.T) {
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
		mRole               *api.MongodbRole
		mRoleBinding        *api.MongodbRoleBinding
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
				},
				Spec: api.MongodbRoleSpec{
					Provider: provider,
				},
			},
			mRoleBinding: &api.MongodbRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: "m-bind",
				},
				Spec: api.MongodbRoleBindingSpec{
					RoleRef: "m-read",
				},
				Status: api.MongodbRoleBindingStatus{
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
			mRole: &api.MongodbRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: "m-read",
				},
				Spec: api.MongodbRoleSpec{
					Provider: provider,
				},
			},
			mRoleBinding: &api.MongodbRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: "m-bind",
				},
				Spec: api.MongodbRoleBindingSpec{
					RoleRef: "m-read",
				},
				Status: api.MongodbRoleBindingStatus{
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
			namespace := "m" + strconv.Itoa(pos)

			vaultCredentialSecret.Namespace = namespace
			test.mRole.Namespace = namespace

			if test.createVaultCred {
				_, err := test.userManger.kubeClient.CoreV1().Secrets(namespace).Create(vaultCredentialSecret)
				assert.Nil(t, err)
			}

			_, err := test.userManger.dbClient.AuthorizationV1alpha1().MongodbRoles(namespace).Create(test.mRole)
			if assert.Nil(t, err) {
				_, err := test.userManger.dbClient.AuthorizationV1alpha1().MongodbRoleBindings(namespace).Create(test.mRoleBinding)
				if assert.Nil(t, err) {
					start := time.Now().Unix()
					test.userManger.runMongodbRoleBindingFinalizer(test.mRoleBinding, test.timeout, test.interval)

					if test.finishBeforeTimeout {
						assert.Condition(t, func() (success bool) {
							if (time.Now().Unix() - start) < int64(test.timeout.Seconds()) {
								return true
							}
							return false
						})

						assert.Condition(t, func() (success bool) {
							if _, ok := test.userManger.processingFinalizer[getMongodbRoleBindingId(test.mRoleBinding)]; !ok {
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
							if _, ok := test.userManger.processingFinalizer[getMongodbRoleBindingId(test.mRoleBinding)]; !ok {
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

func TestUserManagerController_reconcileMongodbRoleBinding(t *testing.T) {
	mRBinding := api.MongodbRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "m-role_binding",
			Namespace: "m",
			UID:       "1234",
		},
		Spec: api.MongodbRoleBindingSpec{
			RoleRef: "test",
			Store: api.Store{
				Secret: "m-cred",
			},
			Subjects: []rbacv1.Subject{
				{
					Namespace: "m",
					Name:      "sa",
					Kind:      rbacv1.ServiceAccountKind,
				},
			},
		},
	}

	testData := []struct {
		testName           string
		dbRBClient         database.DatabaseRoleBindingInterface
		mRBinding          api.MongodbRoleBinding
		expectedErr        bool
		hasStatusCondition bool
		expectedPhase      api.MongodbRoleBindingPhase
	}{
		{
			testName:           "initial stage, on error",
			mRBinding:          mRBinding,
			dbRBClient:         &fakeDRB{},
			expectedErr:        false,
			hasStatusCondition: false,
			expectedPhase:      MongodbRoleBindingPhaseSuccess,
		},
		{
			testName:  "initial stage, failed to get credential",
			mRBinding: mRBinding,
			dbRBClient: &fakeDRB{
				errorOccurredInGetCredential: true,
			},
			expectedErr:        true,
			hasStatusCondition: true,
			expectedPhase:      MongodbRoleBindingPhaseGetCredential,
		},
		{
			testName:  "initial stage, failed to create secret",
			mRBinding: mRBinding,
			dbRBClient: &fakeDRB{
				errorOccurredInCreateSecret: true,
			},
			expectedErr:        true,
			hasStatusCondition: true,
			expectedPhase:      MongodbRoleBindingPhaseCreateSecret,
		},
		{
			testName:  "initial stage, failed to create rbac role",
			mRBinding: mRBinding,
			dbRBClient: &fakeDRB{
				errorOccurredInCreateRole: true,
			},
			expectedErr:        true,
			hasStatusCondition: true,
			expectedPhase:      MongodbRoleBindingPhaseCreateRole,
		},
		{
			testName:  "initial stage, failed to create rbac role binding",
			mRBinding: mRBinding,
			dbRBClient: &fakeDRB{
				errorOccurredInCreateRoleBinding: true,
			},
			expectedErr:        true,
			hasStatusCondition: true,
			expectedPhase:      MongodbRoleBindingPhaseCreateRoleBinding,
		},
		{
			testName: "initial stage, no error, test phase dependency",
			mRBinding: func(p api.MongodbRoleBinding) api.MongodbRoleBinding {
				p.Status.Phase = MongodbRoleBindingPhaseCreateRole
				return p
			}(mRBinding),
			dbRBClient: &fakeDRB{
				errorOccurredInCreateSecret:  true,
				errorOccurredInGetCredential: true,
			},
			expectedErr:        false,
			hasStatusCondition: false,
			expectedPhase:      MongodbRoleBindingPhaseSuccess,
		},
		{
			testName: "update stage, no error",
			mRBinding: func(p api.MongodbRoleBinding) api.MongodbRoleBinding {
				p.Generation = 2
				p.Status.ObservedGeneration = 1
				p.Status.Phase = MongodbRoleBindingPhaseSuccess
				return p
			}(mRBinding),
			dbRBClient:         &fakeDRB{},
			expectedErr:        false,
			hasStatusCondition: false,
			expectedPhase:      MongodbRoleBindingPhaseSuccess,
		},
		{
			testName: "update stage, failed to update rbac role binding",
			mRBinding: func(p api.MongodbRoleBinding) api.MongodbRoleBinding {
				p.Generation = 2
				p.Status.ObservedGeneration = 1
				p.Status.Phase = MongodbRoleBindingPhaseSuccess
				return p
			}(mRBinding),
			dbRBClient: &fakeDRB{
				errorOccurredInUpdateRoleBinding: true,
			},
			expectedErr:        true,
			hasStatusCondition: true,
			expectedPhase:      MongodbRoleBindingPhaseSuccess,
		},
	}

	for _, test := range testData {
		t.Run(test.testName, func(t *testing.T) {
			c := &UserManagerController{
				kubeClient: kfake.NewSimpleClientset(),
				dbClient:   dbfake.NewSimpleClientset(),
			}

			_, err := c.dbClient.AuthorizationV1alpha1().MongodbRoleBindings(test.mRBinding.Namespace).Create(&test.mRBinding)
			if !assert.Nil(t, err) {
				return
			}

			err = c.reconcileMongodbRoleBinding(test.dbRBClient, &test.mRBinding)
			if test.expectedErr {
				if assert.NotNil(t, err) {
					if test.hasStatusCondition {
						p, err2 := c.dbClient.AuthorizationV1alpha1().MongodbRoleBindings(test.mRBinding.Namespace).Get(test.mRBinding.Name, metav1.GetOptions{})
						if assert.Nil(t, err2) {
							assert.Condition(t, func() (success bool) {
								if len(p.Status.Conditions) == 0 {
									return false
								}
								return true
							}, "should have status.conditions")

							assert.Condition(t, func() (success bool) {
								if string(test.expectedPhase) == string(p.Status.Phase) {
									return true
								}
								return false
							}, "check phase")
						}
					}
				}
			} else {
				if assert.Nil(t, err) {
					p, err2 := c.dbClient.AuthorizationV1alpha1().MongodbRoleBindings(test.mRBinding.Namespace).Get(test.mRBinding.Name, metav1.GetOptions{})
					if assert.Nil(t, err2) {
						assert.Condition(t, func() (success bool) {
							if len(p.Status.Conditions) != 0 {
								return false
							}
							return true
						}, "should not have status.conditions")

						assert.Condition(t, func() (success bool) {
							if string(test.expectedPhase) == string(p.Status.Phase) {
								return true
							}
							return false
						}, "check phase")
					}
				}
			}
		})
	}

}
