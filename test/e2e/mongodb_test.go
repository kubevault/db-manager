package e2e_test

import (
	"fmt"
	"time"

	api "github.com/kubedb/user-manager/apis/authorization/v1alpha1"
	"github.com/kubedb/user-manager/test/e2e/framework"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	kerrors "k8s.io/apimachinery/pkg/api/errors"

	patchutil "github.com/kubedb/user-manager/client/clientset/versioned/typed/authorization/v1alpha1/util"
	"github.com/kubedb/user-manager/pkg/vault/database"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Mongodb role and role binding", func() {

	var f *framework.Invocation

	BeforeEach(func() {
		f = root.Invoke()

	})

	AfterEach(func() {
		time.Sleep(20 * time.Second)
	})

	var (
		IsSecretCreated = func(name, namespace string) {
			By(fmt.Sprintf("Waiting for secret (%s/%s) to create", namespace, name))
			Eventually(func() bool {
				_, err := f.KubeClient.CoreV1().Secrets(namespace).Get(name, metav1.GetOptions{})
				if err == nil {
					return true
				}
				return false
			}, timeOut, pollingInterval).Should(BeTrue())
		}

		IsSecretDeleted = func(name, namespace string) {
			By(fmt.Sprintf("Waiting for secret (%s/%s) to delete", namespace, name))
			Eventually(func() bool {
				_, err := f.KubeClient.CoreV1().Secrets(namespace).Get(name, metav1.GetOptions{})
				return kerrors.IsNotFound(err)
			}, timeOut, pollingInterval).Should(BeTrue())
		}

		IsRbacRoleCreated = func(name, namespace string) {
			By(fmt.Sprintf("Waiting for rbac role (%s/%s) to create", namespace, name))
			Eventually(func() bool {
				_, err := f.KubeClient.RbacV1().Roles(namespace).Get(name, metav1.GetOptions{})
				if err == nil {
					return true
				}
				return false
			}, timeOut, pollingInterval).Should(BeTrue())
		}

		IsRbacRoleDeleted = func(name, namespace string) {
			By(fmt.Sprintf("Waiting for rbac role (%s/%s) to delete", namespace, name))
			Eventually(func() bool {
				_, err := f.KubeClient.RbacV1().Roles(namespace).Get(name, metav1.GetOptions{})
				return kerrors.IsNotFound(err)
			}, timeOut, pollingInterval).Should(BeTrue())
		}

		IsRbacRoleBindingCreated = func(name, namespace string) {
			By(fmt.Sprintf("Waiting for rbac role binding (%s/%s) to create", namespace, name))
			Eventually(func() bool {
				_, err := f.KubeClient.RbacV1().RoleBindings(namespace).Get(name, metav1.GetOptions{})
				if err == nil {
					return true
				}
				return false
			}, timeOut, pollingInterval).Should(BeTrue())
		}

		IsRbacRoleBindingDeleted = func(name, namespace string) {
			By(fmt.Sprintf("Waiting for rbac role binding (%s/%s) to delete", namespace, name))
			Eventually(func() bool {
				_, err := f.KubeClient.RbacV1().RoleBindings(namespace).Get(name, metav1.GetOptions{})
				return kerrors.IsNotFound(err)
			}, timeOut, pollingInterval).Should(BeTrue())
		}

		// vault related
		IsVaultDatabaseConfigCreated = func(name string) {
			By(fmt.Sprintf("Checking Is vault database config created"))
			cl, err := f.GetVaultClient()
			Expect(err).NotTo(HaveOccurred(), "Get vault client")

			req := cl.NewRequest("GET", fmt.Sprintf("/v1/database/config/%s", name))
			Eventually(func() bool {
				_, err := cl.RawRequest(req)
				return err == nil
			}, timeOut, pollingInterval).Should(BeTrue(), "Is vault database config created")
		}

		IsVaultDatabaseRoleCreated = func(name string) {
			By(fmt.Sprintf("Checking Is vault database role created"))
			cl, err := f.GetVaultClient()
			Expect(err).NotTo(HaveOccurred(), "Get vault client")

			req := cl.NewRequest("GET", fmt.Sprintf("/v1/database/roles/%s", name))
			Eventually(func() bool {
				_, err := cl.RawRequest(req)
				return err == nil
			}, timeOut, pollingInterval).Should(BeTrue(), "Is vault database role created")
		}

		IsVaultLeaseRevoked = func(dRB database.DatabaseRoleBindingInterface, leaseID string) {
			By(fmt.Sprintf("Checking Is lease revoked"))
			Eventually(func() bool {
				ok, err := dRB.IsLeaseExpired(leaseID)
				return err == nil && ok == true
			}, timeOut, pollingInterval).Should(BeTrue(), "Is lease revoked")
		}

		IsVaultLeaseValid = func(dRB database.DatabaseRoleBindingInterface, leaseID string) {
			By(fmt.Sprintf("Checking Is lease valid"))
			Eventually(func() bool {
				ok, err := dRB.IsLeaseExpired(leaseID)
				return err == nil && ok == false
			}, timeOut, pollingInterval).Should(BeTrue(), "Is lease valid")
		}

		IsVaultDatabaseRoleDeleted = func(name string) {
			By(fmt.Sprintf("Checking Is vault database role deleted"))
			cl, err := f.GetVaultClient()
			Expect(err).NotTo(HaveOccurred(), "Get vault client")

			req := cl.NewRequest("GET", fmt.Sprintf("/v1/database/roles/%s", name))
			Eventually(func() bool {
				_, err := cl.RawRequest(req)
				return err != nil
			}, timeOut, pollingInterval).Should(BeTrue(), "Is vault database role deleted")
		}

		IsMongodbRoleCreated = func(name, namespace string) {
			By(fmt.Sprintf("Checking Is MongodbRole(%s/%s) created", namespace, name))
			Eventually(func() bool {
				_, err := f.DBClient.AuthorizationV1alpha1().MongodbRoles(namespace).Get(name, metav1.GetOptions{})
				return err == nil
			}, timeOut, pollingInterval).Should(BeTrue(), "Is Mongodb role created")
		}

		IsMongodbRoleDeleted = func(name, namespace string) {
			By(fmt.Sprintf("Checking Is MongodbRole(%s/%s) deleted", namespace, name))
			Eventually(func() bool {
				_, err := f.DBClient.AuthorizationV1alpha1().MongodbRoles(namespace).Get(name, metav1.GetOptions{})
				return kerrors.IsNotFound(err)
			}, timeOut, pollingInterval).Should(BeTrue(), "Is MongodbRole role deleted")
		}

		IsMongodbRoleBindingCreated = func(name, namespace string) {
			By(fmt.Sprintf("Checking Is MongodbRoleBinding(%s/%s) created", namespace, name))
			Eventually(func() bool {
				_, err := f.DBClient.AuthorizationV1alpha1().MongodbRoleBindings(namespace).Get(name, metav1.GetOptions{})
				return err == nil
			}, timeOut, pollingInterval).Should(BeTrue(), "Is MongodbRoleBinding created")
		}

		IsMongodbRoleBindingDeleted = func(name, namespace string) {
			By(fmt.Sprintf("Checking Is MongodbRoleBinding(%s/%s) deleted", namespace, name))
			Eventually(func() bool {
				_, err := f.DBClient.AuthorizationV1alpha1().MongodbRoleBindings(namespace).Get(name, metav1.GetOptions{})
				return kerrors.IsNotFound(err)
			}, timeOut, pollingInterval).Should(BeTrue(), "Is MongodbRoleBinding role deleted")
		}
	)

	Describe("MongodbRole", func() {
		var (
			mRole api.MongodbRole
		)

		BeforeEach(func() {
			mRole = api.MongodbRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "m-role-test1",
					Namespace: f.Namespace(),
				},
				Spec: api.MongodbRoleSpec{
					Provider: &api.ProviderSpec{
						Vault: &api.VaultSpec{
							Address:             f.VaultUrl,
							TokenSecret:         framework.VaultTokenSecret,
							SkipTLSVerification: true,
						},
					},
					Database: &api.DatabaseConfigForMongodb{
						Name:             "mongodb-test1",
						CredentialSecret: framework.MongodbCredentialSecret,
						ConnectionUrl:    fmt.Sprintf("mongodb://{{username}}:{{password}}@%s/admin?ssl=false", f.MongodbUrl),
						AllowedRoles:     "*",
					},
					DBName: "mongodb-test1",
					CreationStatements: []string{
						"{ \"db\": \"admin\", \"roles\": [{ \"role\": \"readWrite\" }, {\"role\": \"read\", \"db\": \"foo\"}] }",
					},
					MaxTTL:     "1h",
					DefaultTTL: "300",
				},
			}
		})

		Context("Create MongodbRole", func() {
			var p api.MongodbRole

			BeforeEach(func() {
				p = mRole
			})

			AfterEach(func() {
				err := f.DBClient.AuthorizationV1alpha1().MongodbRoles(p.Namespace).Delete(p.Name, &metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred(), "Delete MongodbRole")

				IsMongodbRoleDeleted(p.Name, p.Namespace)
				IsVaultDatabaseRoleDeleted(p.Name)
			})

			It("should be successful", func() {
				_, err := f.DBClient.AuthorizationV1alpha1().MongodbRoles(mRole.Namespace).Create(&p)
				Expect(err).NotTo(HaveOccurred(), "Create Mongodbole")

				IsVaultDatabaseConfigCreated(p.Spec.Database.Name)
				IsVaultDatabaseRoleCreated(p.Name)
			})
		})

		Context("Delete MongodbRole, invalid vault address", func() {
			var p api.MongodbRole

			BeforeEach(func() {
				p = mRole
				p.Spec.Provider.Vault.Address = "http://invalid.com:8200"

				_, err := f.DBClient.AuthorizationV1alpha1().MongodbRoles(mRole.Namespace).Create(&p)
				Expect(err).NotTo(HaveOccurred(), "Create MongodbRole")

				IsMongodbRoleCreated(p.Name, p.Namespace)
			})

			It("should be successful", func() {
				err := f.DBClient.AuthorizationV1alpha1().MongodbRoles(p.Namespace).Delete(p.Name, &metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred(), "Delete MongodbRole")

				IsMongodbRoleDeleted(p.Name, p.Namespace)
			})
		})

	})

	Describe("MongodbRoleBinding", func() {
		var (
			mRole api.MongodbRole
		)

		BeforeEach(func() {
			mRole = api.MongodbRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mdb-role-test1",
					Namespace: f.Namespace(),
				},
				Spec: api.MongodbRoleSpec{
					Provider: &api.ProviderSpec{
						Vault: &api.VaultSpec{
							Address:             f.VaultUrl,
							TokenSecret:         framework.VaultTokenSecret,
							SkipTLSVerification: true,
						},
					},
					Database: &api.DatabaseConfigForMongodb{
						Name:             "mongodb-test1",
						CredentialSecret: framework.MongodbCredentialSecret,
						ConnectionUrl:    fmt.Sprintf("mongodb://{{username}}:{{password}}@%s/admin?ssl=false", f.MongodbUrl),
						AllowedRoles:     "*",
					},
					DBName: "mongodb-test1",
					CreationStatements: []string{
						"{ \"db\": \"admin\", \"roles\": [{ \"role\": \"readWrite\" }, {\"role\": \"read\", \"db\": \"foo\"}] }",
					},
					MaxTTL:     "1h",
					DefaultTTL: "300",
				},
			}

			_, err := f.DBClient.AuthorizationV1alpha1().MongodbRoles(mRole.Namespace).Create(&mRole)
			Expect(err).NotTo(HaveOccurred(), "Create MongodbRole")

			IsMongodbRoleCreated(mRole.Name, mRole.Namespace)
		})

		AfterEach(func() {
			err := f.DBClient.AuthorizationV1alpha1().MongodbRoles(mRole.Namespace).Delete(mRole.Name, &metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred(), "Delete MongodbRole")

			IsMongodbRoleDeleted(mRole.Name, mRole.Namespace)
			IsVaultDatabaseRoleDeleted(mRole.Name)
		})

		Context("Create", func() {
			var mRoleBinding *api.MongodbRoleBinding
			BeforeEach(func() {
				mRoleBinding = &api.MongodbRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "mdb-read",
						Namespace: f.Namespace(),
					},
					Spec: api.MongodbRoleBindingSpec{
						RoleRef: mRole.Name,
						Subjects: []rbacv1.Subject{
							{
								Name:      "mdb-sa",
								Kind:      rbacv1.ServiceAccountKind,
								Namespace: f.Namespace(),
							},
						},
						Store: api.Store{
							Secret: "mdb-cred",
						},
					},
				}
			})

			AfterEach(func() {
				err := f.DBClient.AuthorizationV1alpha1().MongodbRoleBindings(mRoleBinding.Namespace).Delete(mRoleBinding.Name, &metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred(), "Delete MongodbRoleBindings")

				IsMongodbRoleBindingDeleted(mRoleBinding.Name, mRoleBinding.Namespace)

				IsSecretDeleted(mRoleBinding.Spec.Store.Secret, mRoleBinding.Namespace)
				IsRbacRoleDeleted(fmt.Sprintf("mongodbrolebinding-%s-credential-reader", mRoleBinding.Name), mRoleBinding.Namespace)
				IsRbacRoleBindingDeleted(fmt.Sprintf("mongodbrolebinding-%s-credential-reader", mRoleBinding.Name), mRoleBinding.Namespace)
			})

			It("should be successful", func() {
				_, err := f.DBClient.AuthorizationV1alpha1().MongodbRoleBindings(mRoleBinding.Namespace).Create(mRoleBinding)
				Expect(err).NotTo(HaveOccurred(), "Create PostgresRoleBinding")

				IsMongodbRoleBindingCreated(mRoleBinding.Name, mRoleBinding.Namespace)

				IsSecretCreated(mRoleBinding.Spec.Store.Secret, mRoleBinding.Namespace)

				IsSecretCreated(mRoleBinding.Spec.Store.Secret, mRoleBinding.Namespace)
				IsRbacRoleCreated(fmt.Sprintf("mongodbrolebinding-%s-credential-reader", mRoleBinding.Name), mRoleBinding.Namespace)
				IsRbacRoleBindingCreated(fmt.Sprintf("mongodbrolebinding-%s-credential-reader", mRoleBinding.Name), mRoleBinding.Namespace)
			})
		})
	})

	Describe("Lease revoke and reissue", func() {
		var (
			mRole        api.MongodbRole
			mRoleBinding api.MongodbRoleBinding
		)

		BeforeEach(func() {
			mRole = api.MongodbRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mdb-role-test1",
					Namespace: f.Namespace(),
				},
				Spec: api.MongodbRoleSpec{
					Provider: &api.ProviderSpec{
						Vault: &api.VaultSpec{
							Address:             f.VaultUrl,
							TokenSecret:         framework.VaultTokenSecret,
							SkipTLSVerification: true,
						},
					},
					Database: &api.DatabaseConfigForMongodb{
						Name:             "mongodb-test1",
						CredentialSecret: framework.MongodbCredentialSecret,
						ConnectionUrl:    fmt.Sprintf("mongodb://{{username}}:{{password}}@%s/admin?ssl=false", f.MongodbUrl),
						AllowedRoles:     "*",
					},
					DBName: "mongodb-test1",
					CreationStatements: []string{
						"{ \"db\": \"admin\", \"roles\": [{ \"role\": \"readWrite\" }, {\"role\": \"read\", \"db\": \"foo\"}] }",
					},
					MaxTTL:     "1h",
					DefaultTTL: "300",
				},
			}

			mRoleBinding = api.MongodbRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mdb-read",
					Namespace: f.Namespace(),
				},
				Spec: api.MongodbRoleBindingSpec{
					RoleRef: mRole.Name,
					Subjects: []rbacv1.Subject{
						{
							Name:      "mdb-sa",
							Kind:      rbacv1.ServiceAccountKind,
							Namespace: f.Namespace(),
						},
					},
					Store: api.Store{
						Secret: "mdb-cred",
					},
				},
			}
		})

		Context("for MongodbRole and MongodbRoleBinding", func() {
			BeforeEach(func() {
				_, err := f.DBClient.AuthorizationV1alpha1().MongodbRoles(mRole.Namespace).Create(&mRole)
				Expect(err).NotTo(HaveOccurred(), "Create MongodbRole")
				IsMongodbRoleCreated(mRole.Name, mRole.Namespace)

				_, err = f.DBClient.AuthorizationV1alpha1().MongodbRoleBindings(mRoleBinding.Namespace).Create(&mRoleBinding)
				Expect(err).NotTo(HaveOccurred(), "Create MongodbRoleBinding")
				IsMongodbRoleBindingCreated(mRoleBinding.Name, mRoleBinding.Namespace)
				IsSecretCreated(mRoleBinding.Spec.Store.Secret, mRoleBinding.Namespace)
				IsSecretCreated(mRoleBinding.Spec.Store.Secret, mRoleBinding.Namespace)
				IsRbacRoleCreated(fmt.Sprintf("mongodbrolebinding-%s-credential-reader", mRoleBinding.Name), mRoleBinding.Namespace)
				IsRbacRoleBindingCreated(fmt.Sprintf("mongodbrolebinding-%s-credential-reader", mRoleBinding.Name), mRoleBinding.Namespace)
			})

			AfterEach(func() {
				err := f.DBClient.AuthorizationV1alpha1().MongodbRoles(mRole.Namespace).Delete(mRole.Name, &metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred(), "Delete MongodbRole")

				IsMongodbRoleDeleted(mRole.Name, mRole.Namespace)
				IsVaultDatabaseRoleDeleted(mRole.Name)

				err = f.DBClient.AuthorizationV1alpha1().MongodbRoleBindings(mRoleBinding.Namespace).Delete(mRoleBinding.Name, &metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred(), "Delete MongodbRoleBindings")

				IsMongodbRoleBindingDeleted(mRoleBinding.Name, mRoleBinding.Namespace)

				IsSecretDeleted(mRoleBinding.Spec.Store.Secret, mRoleBinding.Namespace)
				IsRbacRoleDeleted(fmt.Sprintf("Mongodbrolebinding-%s-credential-reader", mRoleBinding.Name), mRoleBinding.Namespace)
				IsRbacRoleBindingDeleted(fmt.Sprintf("Mongodbrolebinding-%s-credential-reader", mRoleBinding.Name), mRoleBinding.Namespace)

			})

			It("delete role should revoke lease successfully, recreate role should reissue lease successfully", func() {
				mRB, err := f.DBClient.AuthorizationV1alpha1().MongodbRoleBindings(mRoleBinding.Namespace).Get(mRoleBinding.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred(), "Get MongodbRoleBinding")
				Expect(mRB.Status.Lease.ID != "").To(BeTrue(), "status.lease.id should be non empty")
				previousLease := mRB.Status.Lease

				dRB, err := database.NewDatabaseRoleBindingForMongodb(f.KubeClient, f.DBClient, mRB)
				Expect(err).NotTo(HaveOccurred())

				// delete role
				err = f.DBClient.AuthorizationV1alpha1().MongodbRoles(mRole.Namespace).Delete(mRole.Name, &metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred(), "Delete MongodbRole")
				IsMongodbRoleDeleted(mRole.Name, mRole.Namespace)

				IsVaultLeaseRevoked(dRB, previousLease.ID)

				// recreate role
				_, err = f.DBClient.AuthorizationV1alpha1().MongodbRoles(mRole.Namespace).Create(&mRole)
				Expect(err).NotTo(HaveOccurred(), "Create MongodbRole")
				IsMongodbRoleCreated(mRole.Name, mRole.Namespace)

				Eventually(func() bool {
					mRB, err = f.DBClient.AuthorizationV1alpha1().MongodbRoleBindings(mRoleBinding.Namespace).Get(mRoleBinding.Name, metav1.GetOptions{})
					return err == nil && mRB.Status.Lease.ID != ""
				}, timeOut, pollingInterval).Should(BeTrue(), "MongodbRoleBinding status.lease.id should be non empty")

				curLease := mRB.Status.Lease
				IsVaultLeaseValid(dRB, curLease.ID)

				sr, err := f.KubeClient.CoreV1().Secrets(mRoleBinding.Namespace).Get(mRoleBinding.Spec.Store.Secret, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred(), "Get secret")
				Expect(sr.Data != nil &&
					string(sr.Data["lease_id"]) == curLease.ID).To(BeTrue(), "lease in the secret should be updated")
			})

			It("update role should revoke the previous lease and issue a new lease successfully", func() {
				mRB, err := f.DBClient.AuthorizationV1alpha1().MongodbRoleBindings(mRoleBinding.Namespace).Get(mRoleBinding.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred(), "Get MongodbRoleBinding")
				Expect(mRB.Status.Lease.ID != "").To(BeTrue(), "status.lease.id should be non empty")
				previousLease := mRB.Status.Lease

				dRB, err := database.NewDatabaseRoleBindingForMongodb(f.KubeClient, f.DBClient, mRB)
				Expect(err).NotTo(HaveOccurred())

				// update role
				_, _, err = patchutil.PatchMongodbRole(f.DBClient.AuthorizationV1alpha1(), &mRole, func(r *api.MongodbRole) *api.MongodbRole {
					r.Spec.DefaultTTL = "500"
					return r
				})
				Expect(err).NotTo(HaveOccurred(), "Update MongodbRole")
				IsVaultLeaseRevoked(dRB, previousLease.ID)

				Eventually(func() bool {
					mRB, err = f.DBClient.AuthorizationV1alpha1().MongodbRoleBindings(mRoleBinding.Namespace).Get(mRoleBinding.Name, metav1.GetOptions{})
					return err == nil &&
						mRB.Status.Lease.ID != "" &&
						mRB.Status.Lease.ID != previousLease.ID &&
						mRB.Status.Lease.Duration == 500
				}, timeOut, pollingInterval).Should(BeTrue(), "MongodbRoleBinding status.lease.id should be reissued")

				curLease := mRB.Status.Lease
				IsVaultLeaseValid(dRB, curLease.ID)

				sr, err := f.KubeClient.CoreV1().Secrets(mRoleBinding.Namespace).Get(mRoleBinding.Spec.Store.Secret, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred(), "Get secret")
				Expect(sr.Data != nil &&
					string(sr.Data["lease_id"]) == curLease.ID).To(BeTrue(), "lease in the secret should be updated")
			})
		})
	})

})
