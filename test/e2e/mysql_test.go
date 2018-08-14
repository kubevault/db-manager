package e2e_test

import (
	"fmt"
	"time"

	api "github.com/kubedb/user-manager/apis/authorization/v1alpha1"
	"github.com/kubedb/user-manager/test/e2e/framework"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	kerrors "k8s.io/apimachinery/pkg/api/errors"

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
					Database: &api.DatabaseSpec{
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
					Database: &api.DatabaseSpec{
						Name:             "mysql-test1",
						PluginName: "mysql-rds-database-plugin",
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
				Expect(err).NotTo(HaveOccurred(), "Create PostgresRoleBinding")

				IsMysqlRoleBindingCreated(mRoleBinding.Name, mRoleBinding.Namespace)

				IsSecretCreated(mRoleBinding.Spec.Store.Secret, mRoleBinding.Namespace)

				IsSecretCreated(mRoleBinding.Spec.Store.Secret, mRoleBinding.Namespace)
				IsRbacRoleCreated(fmt.Sprintf("mysqlrolebinding-%s-credential-reader", mRoleBinding.Name), mRoleBinding.Namespace)
				IsRbacRoleBindingCreated(fmt.Sprintf("mysqlrolebinding-%s-credential-reader", mRoleBinding.Name), mRoleBinding.Namespace)
			})
		})
	})

})
