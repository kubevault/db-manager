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

var _ = Describe("Mysql role and role binding", func() {

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

		IsMysqlRoleCreated = func(name, namespace string) {
			By(fmt.Sprintf("Checking Is MysqlRole(%s/%s) created", namespace, name))
			Eventually(func() bool {
				_, err := f.DBClient.AuthorizationV1alpha1().MysqlRoles(namespace).Get(name, metav1.GetOptions{})
				return err == nil
			}, timeOut, pollingInterval).Should(BeTrue(), "Is Mysql role created")
		}

		IsMysqlRoleDeleted = func(name, namespace string) {
			By(fmt.Sprintf("Checking Is MysqlRole(%s/%s) deleted", namespace, name))
			Eventually(func() bool {
				_, err := f.DBClient.AuthorizationV1alpha1().MysqlRoles(namespace).Get(name, metav1.GetOptions{})
				return kerrors.IsNotFound(err)
			}, timeOut, pollingInterval).Should(BeTrue(), "Is MysqlRole role deleted")
		}

		IsMysqlRoleBindingCreated = func(name, namespace string) {
			By(fmt.Sprintf("Checking Is MysqlRoleBinding(%s/%s) created", namespace, name))
			Eventually(func() bool {
				_, err := f.DBClient.AuthorizationV1alpha1().MysqlRoleBindings(namespace).Get(name, metav1.GetOptions{})
				return err == nil
			}, timeOut, pollingInterval).Should(BeTrue(), "Is MysqlRoleBinding created")
		}

		IsMysqlRoleBindingDeleted = func(name, namespace string) {
			By(fmt.Sprintf("Checking Is MysqlRoleBinding(%s/%s) deleted", namespace, name))
			Eventually(func() bool {
				_, err := f.DBClient.AuthorizationV1alpha1().MysqlRoleBindings(namespace).Get(name, metav1.GetOptions{})
				return kerrors.IsNotFound(err)
			}, timeOut, pollingInterval).Should(BeTrue(), "Is MysqlRoleBinding role deleted")
		}
	)

	Describe("MysqlRole", func() {
		var (
			mRole api.MysqlRole
		)

		BeforeEach(func() {
			mRole = api.MysqlRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "m-role-test1",
					Namespace: f.Namespace(),
				},
				Spec: api.MysqlRoleSpec{
					Provider: &api.ProviderSpec{
						Vault: &api.VaultSpec{
							Address:             f.VaultUrl,
							TokenSecret:         framework.VaultTokenSecret,
							SkipTLSVerification: true,
						},
					},
					Database: &api.DatabaseConfigForMysql{
						Name:             "mysql-test1",
						CredentialSecret: framework.MysqlCredentialSecret,
						ConnectionUrl:    fmt.Sprintf("{{username}}:{{password}}@tcp(%s)/", f.MysqlUrl),
						AllowedRoles:     "*",
					},
					DBName: "mysql-test1",
					CreationStatements: []string{
						"CREATE USER '{{name}}'@'%' IDENTIFIED BY '{{password}}';",
						"GRANT SELECT ON *.* TO '{{name}}'@'%';",
					},
					MaxTTL:     "1h",
					DefaultTTL: "300",
				},
			}
		})

		Context("Create MysqlRole", func() {
			var p api.MysqlRole

			BeforeEach(func() {
				p = mRole
			})

			AfterEach(func() {
				err := f.DBClient.AuthorizationV1alpha1().MysqlRoles(p.Namespace).Delete(p.Name, &metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred(), "Delete MysqlRole")

				IsMysqlRoleDeleted(p.Name, p.Namespace)
				IsVaultDatabaseRoleDeleted(p.Name)
			})

			It("should be successful", func() {
				_, err := f.DBClient.AuthorizationV1alpha1().MysqlRoles(mRole.Namespace).Create(&p)
				Expect(err).NotTo(HaveOccurred(), "Create Mysqlole")

				IsVaultDatabaseConfigCreated(p.Spec.Database.Name)
				IsVaultDatabaseRoleCreated(p.Name)
			})
		})

		Context("Delete MysqlRole, invalid vault address", func() {
			var p api.MysqlRole

			BeforeEach(func() {
				p = mRole
				p.Spec.Provider.Vault.Address = "http://invalid.com:8200"

				_, err := f.DBClient.AuthorizationV1alpha1().MysqlRoles(mRole.Namespace).Create(&p)
				Expect(err).NotTo(HaveOccurred(), "Create MysqlRole")

				IsMysqlRoleCreated(p.Name, p.Namespace)
			})

			It("should be successful", func() {
				err := f.DBClient.AuthorizationV1alpha1().MysqlRoles(p.Namespace).Delete(p.Name, &metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred(), "Delete MysqlRole")

				IsMysqlRoleDeleted(p.Name, p.Namespace)
			})
		})

	})

	Describe("MysqlRoleBinding", func() {
		var (
			mRole api.MysqlRole
		)

		BeforeEach(func() {
			mRole = api.MysqlRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "m-role-test1",
					Namespace: f.Namespace(),
				},
				Spec: api.MysqlRoleSpec{
					Provider: &api.ProviderSpec{
						Vault: &api.VaultSpec{
							Address:             f.VaultUrl,
							TokenSecret:         framework.VaultTokenSecret,
							SkipTLSVerification: true,
						},
					},
					Database: &api.DatabaseConfigForMysql{
						Name:             "mysql-test1",
						PluginName:       "mysql-rds-database-plugin",
						CredentialSecret: framework.MysqlCredentialSecret,
						ConnectionUrl:    fmt.Sprintf("{{username}}:{{password}}@tcp(%s)/", f.MysqlUrl),
						AllowedRoles:     "*",
					},
					DBName: "mysql-test1",
					CreationStatements: []string{
						"CREATE USER '{{name}}'@'%' IDENTIFIED BY '{{password}}';",
						"GRANT SELECT ON *.* TO '{{name}}'@'%';",
					},
					MaxTTL:     "1h",
					DefaultTTL: "300",
				},
			}

			_, err := f.DBClient.AuthorizationV1alpha1().MysqlRoles(mRole.Namespace).Create(&mRole)
			Expect(err).NotTo(HaveOccurred(), "Create MysqlRole")

			IsMysqlRoleCreated(mRole.Name, mRole.Namespace)
		})

		AfterEach(func() {
			err := f.DBClient.AuthorizationV1alpha1().MysqlRoles(mRole.Namespace).Delete(mRole.Name, &metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred(), "Delete MysqlRole")

			IsMysqlRoleDeleted(mRole.Name, mRole.Namespace)
			IsVaultDatabaseRoleDeleted(mRole.Name)
		})

		Context("Create", func() {
			var mRoleBinding *api.MysqlRoleBinding
			BeforeEach(func() {
				mRoleBinding = &api.MysqlRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "m-read",
						Namespace: f.Namespace(),
					},
					Spec: api.MysqlRoleBindingSpec{
						RoleRef: mRole.Name,
						Subjects: []rbacv1.Subject{
							{
								Name:      "m-sa",
								Kind:      rbacv1.ServiceAccountKind,
								Namespace: f.Namespace(),
							},
						},
						Store: api.Store{
							Secret: "m-cred",
						},
					},
				}
			})

			AfterEach(func() {
				err := f.DBClient.AuthorizationV1alpha1().MysqlRoleBindings(mRoleBinding.Namespace).Delete(mRoleBinding.Name, &metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred(), "Delete MysqlRoleBindings")

				IsMysqlRoleBindingDeleted(mRoleBinding.Name, mRoleBinding.Namespace)

				IsSecretDeleted(mRoleBinding.Spec.Store.Secret, mRoleBinding.Namespace)
				IsRbacRoleDeleted(fmt.Sprintf("mysqlrolebinding-%s-credential-reader", mRoleBinding.Name), mRoleBinding.Namespace)
				IsRbacRoleBindingDeleted(fmt.Sprintf("mysqlrolebinding-%s-credential-reader", mRoleBinding.Name), mRoleBinding.Namespace)
			})

			It("should be successful", func() {
				_, err := f.DBClient.AuthorizationV1alpha1().MysqlRoleBindings(mRoleBinding.Namespace).Create(mRoleBinding)
				Expect(err).NotTo(HaveOccurred(), "Create MysqlRoleBinding")

				IsMysqlRoleBindingCreated(mRoleBinding.Name, mRoleBinding.Namespace)

				IsSecretCreated(mRoleBinding.Spec.Store.Secret, mRoleBinding.Namespace)

				IsSecretCreated(mRoleBinding.Spec.Store.Secret, mRoleBinding.Namespace)
				IsRbacRoleCreated(fmt.Sprintf("mysqlrolebinding-%s-credential-reader", mRoleBinding.Name), mRoleBinding.Namespace)
				IsRbacRoleBindingCreated(fmt.Sprintf("mysqlrolebinding-%s-credential-reader", mRoleBinding.Name), mRoleBinding.Namespace)
			})
		})
	})

	Describe("Lease revoke and reissue", func() {
		var (
			mRole        api.MysqlRole
			mRoleBinding api.MysqlRoleBinding
		)

		BeforeEach(func() {
			mRole = api.MysqlRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "m-role-test1",
					Namespace: f.Namespace(),
				},
				Spec: api.MysqlRoleSpec{
					Provider: &api.ProviderSpec{
						Vault: &api.VaultSpec{
							Address:             f.VaultUrl,
							TokenSecret:         framework.VaultTokenSecret,
							SkipTLSVerification: true,
						},
					},
					Database: &api.DatabaseConfigForMysql{
						Name:             "mysql-test1",
						PluginName:       "mysql-rds-database-plugin",
						CredentialSecret: framework.MysqlCredentialSecret,
						ConnectionUrl:    fmt.Sprintf("{{username}}:{{password}}@tcp(%s)/", f.MysqlUrl),
						AllowedRoles:     "*",
					},
					DBName: "mysql-test1",
					CreationStatements: []string{
						"CREATE USER '{{name}}'@'%' IDENTIFIED BY '{{password}}';",
						"GRANT SELECT ON *.* TO '{{name}}'@'%';",
					},
					MaxTTL:     "1h",
					DefaultTTL: "300",
				},
			}

			mRoleBinding = api.MysqlRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "m-read",
					Namespace: f.Namespace(),
				},
				Spec: api.MysqlRoleBindingSpec{
					RoleRef: mRole.Name,
					Subjects: []rbacv1.Subject{
						{
							Name:      "m-sa",
							Kind:      rbacv1.ServiceAccountKind,
							Namespace: f.Namespace(),
						},
					},
					Store: api.Store{
						Secret: "m-cred",
					},
				},
			}
		})

		Context("for mysqlRole and mysqlRoleBinding", func() {
			BeforeEach(func() {
				_, err := f.DBClient.AuthorizationV1alpha1().MysqlRoles(mRole.Namespace).Create(&mRole)
				Expect(err).NotTo(HaveOccurred(), "Create MysqlRole")
				IsMysqlRoleCreated(mRole.Name, mRole.Namespace)

				_, err = f.DBClient.AuthorizationV1alpha1().MysqlRoleBindings(mRoleBinding.Namespace).Create(&mRoleBinding)
				Expect(err).NotTo(HaveOccurred(), "Create MysqlRoleBinding")
				IsMysqlRoleBindingCreated(mRoleBinding.Name, mRoleBinding.Namespace)
				IsSecretCreated(mRoleBinding.Spec.Store.Secret, mRoleBinding.Namespace)
				IsSecretCreated(mRoleBinding.Spec.Store.Secret, mRoleBinding.Namespace)
				IsRbacRoleCreated(fmt.Sprintf("mysqlrolebinding-%s-credential-reader", mRoleBinding.Name), mRoleBinding.Namespace)
				IsRbacRoleBindingCreated(fmt.Sprintf("mysqlrolebinding-%s-credential-reader", mRoleBinding.Name), mRoleBinding.Namespace)
			})

			AfterEach(func() {
				err := f.DBClient.AuthorizationV1alpha1().MysqlRoles(mRole.Namespace).Delete(mRole.Name, &metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred(), "Delete MysqlRole")

				IsMysqlRoleDeleted(mRole.Name, mRole.Namespace)
				IsVaultDatabaseRoleDeleted(mRole.Name)

				err = f.DBClient.AuthorizationV1alpha1().MysqlRoleBindings(mRoleBinding.Namespace).Delete(mRoleBinding.Name, &metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred(), "Delete MysqlRoleBindings")

				IsMysqlRoleBindingDeleted(mRoleBinding.Name, mRoleBinding.Namespace)

				IsSecretDeleted(mRoleBinding.Spec.Store.Secret, mRoleBinding.Namespace)
				IsRbacRoleDeleted(fmt.Sprintf("mysqlrolebinding-%s-credential-reader", mRoleBinding.Name), mRoleBinding.Namespace)
				IsRbacRoleBindingDeleted(fmt.Sprintf("mysqlrolebinding-%s-credential-reader", mRoleBinding.Name), mRoleBinding.Namespace)

			})

			It("delete role should revoke lease successfully, recreate role should reissue lease successfully", func() {
				mRB, err := f.DBClient.AuthorizationV1alpha1().MysqlRoleBindings(mRoleBinding.Namespace).Get(mRoleBinding.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred(), "Get MysqlRoleBinding")
				Expect(mRB.Status.Lease.ID != "").To(BeTrue(), "status.lease.id should be non empty")
				previousLease := mRB.Status.Lease

				dRB, err := database.NewDatabaseRoleBindingForMysql(f.KubeClient, f.DBClient, mRB)
				Expect(err).NotTo(HaveOccurred())

				// delete role
				err = f.DBClient.AuthorizationV1alpha1().MysqlRoles(mRole.Namespace).Delete(mRole.Name, &metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred(), "Delete MysqlRole")
				IsMysqlRoleDeleted(mRole.Name, mRole.Namespace)

				IsVaultLeaseRevoked(dRB, previousLease.ID)

				// recreate role
				_, err = f.DBClient.AuthorizationV1alpha1().MysqlRoles(mRole.Namespace).Create(&mRole)
				Expect(err).NotTo(HaveOccurred(), "Create MysqlRole")
				IsMysqlRoleCreated(mRole.Name, mRole.Namespace)

				Eventually(func() bool {
					mRB, err = f.DBClient.AuthorizationV1alpha1().MysqlRoleBindings(mRoleBinding.Namespace).Get(mRoleBinding.Name, metav1.GetOptions{})
					return err == nil && mRB.Status.Lease.ID != ""
				}, timeOut, pollingInterval).Should(BeTrue(), "MysqlRoleBinding status.lease.id should be non empty")

				curLease := mRB.Status.Lease
				IsVaultLeaseValid(dRB, curLease.ID)

				sr, err := f.KubeClient.CoreV1().Secrets(mRoleBinding.Namespace).Get(mRoleBinding.Spec.Store.Secret, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred(), "Get secret")
				Expect(sr.Data != nil &&
					string(sr.Data["lease_id"]) == curLease.ID).To(BeTrue(), "lease in the secret should be updated")
			})

			It("update role should revoke the previous lease and issue a new lease successfully", func() {
				mRB, err := f.DBClient.AuthorizationV1alpha1().MysqlRoleBindings(mRoleBinding.Namespace).Get(mRoleBinding.Name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred(), "Get MysqlRoleBinding")
				Expect(mRB.Status.Lease.ID != "").To(BeTrue(), "status.lease.id should be non empty")
				previousLease := mRB.Status.Lease

				dRB, err := database.NewDatabaseRoleBindingForMysql(f.KubeClient, f.DBClient, mRB)
				Expect(err).NotTo(HaveOccurred())

				// update role
				_, _, err = patchutil.PatchMysqlRole(f.DBClient.AuthorizationV1alpha1(), &mRole, func(r *api.MysqlRole) *api.MysqlRole {
					r.Spec.DefaultTTL = "500"
					return r
				})
				Expect(err).NotTo(HaveOccurred(), "Update MysqlRole")
				IsVaultLeaseRevoked(dRB, previousLease.ID)

				Eventually(func() bool {
					mRB, err = f.DBClient.AuthorizationV1alpha1().MysqlRoleBindings(mRoleBinding.Namespace).Get(mRoleBinding.Name, metav1.GetOptions{})
					return err == nil &&
						mRB.Status.Lease.ID != "" &&
						mRB.Status.Lease.ID != previousLease.ID &&
						mRB.Status.Lease.Duration == 500
				}, timeOut, pollingInterval).Should(BeTrue(), "MysqlRoleBinding status.lease.id should be reissued")

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
