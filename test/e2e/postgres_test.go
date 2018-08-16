package e2e_test

import (
	"fmt"
	"time"

	api "github.com/kubedb/user-manager/apis/authorization/v1alpha1"
	"github.com/kubedb/user-manager/test/e2e/framework"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	timeOut         = 20 * time.Minute
	pollingInterval = 10 * time.Second
)

var _ = Describe("Postgres role and role binding", func() {

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

		IsPostgresRoleCreated = func(name, namespace string) {
			By(fmt.Sprintf("Checking Is PostgresRole(%s/%s) created", namespace, name))
			Eventually(func() bool {
				_, err := f.DBClient.AuthorizationV1alpha1().PostgresRoles(namespace).Get(name, metav1.GetOptions{})
				return err == nil
			}, timeOut, pollingInterval).Should(BeTrue(), "Is PostgresRole role created")
		}

		IsPostgresRoleDeleted = func(name, namespace string) {
			By(fmt.Sprintf("Checking Is PostgresRole(%s/%s) deleted", namespace, name))
			Eventually(func() bool {
				_, err := f.DBClient.AuthorizationV1alpha1().PostgresRoles(namespace).Get(name, metav1.GetOptions{})
				return kerrors.IsNotFound(err)
			}, timeOut, pollingInterval).Should(BeTrue(), "Is PostgresRole role deleted")
		}

		IsPostgresRoleBindingCreated = func(name, namespace string) {
			By(fmt.Sprintf("Checking Is PostgresRoleBinding(%s/%s) created", namespace, name))
			Eventually(func() bool {
				_, err := f.DBClient.AuthorizationV1alpha1().PostgresRoleBindings(namespace).Get(name, metav1.GetOptions{})
				return err == nil
			}, timeOut, pollingInterval).Should(BeTrue(), "Is PostgresRoleBinding role created")
		}

		IsPostgresRoleBindingDeleted = func(name, namespace string) {
			By(fmt.Sprintf("Checking Is PostgresRoleBinding(%s/%s) deleted", namespace, name))
			Eventually(func() bool {
				_, err := f.DBClient.AuthorizationV1alpha1().PostgresRoleBindings(namespace).Get(name, metav1.GetOptions{})
				return kerrors.IsNotFound(err)
			}, timeOut, pollingInterval).Should(BeTrue(), "Is PostgresRoleBinding role deleted")
		}
	)

	Describe("PostgresRole", func() {
		var (
			pgRole api.PostgresRole
		)

		BeforeEach(func() {
			pgRole = api.PostgresRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pg-role-test1",
					Namespace: f.Namespace(),
				},
				Spec: api.PostgresRoleSpec{
					Provider: &api.ProviderSpec{
						Vault: &api.VaultSpec{
							Address:             f.VaultUrl,
							TokenSecret:         framework.VaultTokenSecret,
							SkipTLSVerification: true,
						},
					},
					Database: &api.DatabaseConfigForPostgres{
						Name:             "postgres-test1",
						CredentialSecret: framework.PostgresCredentialSecret,
						ConnectionUrl:    fmt.Sprintf("postgresql://{{username}}:{{password}}@%s/postgres?sslmode=disable", f.PostgresUrl),
						AllowedRoles:     "*",
					},
					DBName: "postgres-test1",
					CreationStatements: []string{
						"CREATE ROLE \"{{name}}\" WITH LOGIN PASSWORD '{{password}}' VALID UNTIL '{{expiration}}';",
						"GRANT SELECT ON ALL TABLES IN SCHEMA public TO \"{{name}}\";",
					},
					MaxTTL:     "1h",
					DefaultTTL: "300",
				},
			}
		})

		Context("Create PostgresRole", func() {
			var p api.PostgresRole

			BeforeEach(func() {
				p = pgRole
			})

			AfterEach(func() {
				err := f.DBClient.AuthorizationV1alpha1().PostgresRoles(p.Namespace).Delete(p.Name, &metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred(), "Delete PostgresRole")

				IsPostgresRoleDeleted(p.Name, p.Namespace)
				IsVaultDatabaseRoleDeleted(p.Name)
			})

			It("should be successful", func() {
				_, err := f.DBClient.AuthorizationV1alpha1().PostgresRoles(pgRole.Namespace).Create(&p)
				Expect(err).NotTo(HaveOccurred(), "Create PostgresRole")

				IsVaultDatabaseConfigCreated(p.Spec.Database.Name)
				IsVaultDatabaseRoleCreated(p.Name)
			})
		})

		Context("Delete PostgresRole, invalid vault address", func() {
			var p api.PostgresRole

			BeforeEach(func() {
				p = pgRole
				p.Spec.Provider.Vault.Address = "http://invalid.com:8200"

				_, err := f.DBClient.AuthorizationV1alpha1().PostgresRoles(pgRole.Namespace).Create(&p)
				Expect(err).NotTo(HaveOccurred(), "Create PostgresRole")

				IsPostgresRoleCreated(p.Name, p.Namespace)
			})

			It("should be successful", func() {
				err := f.DBClient.AuthorizationV1alpha1().PostgresRoles(p.Namespace).Delete(p.Name, &metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred(), "Delete PostgresRole")

				IsPostgresRoleDeleted(p.Name, p.Namespace)
			})
		})

	})

	Describe("PostgresRoleBinding", func() {
		var (
			pgRole api.PostgresRole
		)

		BeforeEach(func() {
			pgRole = api.PostgresRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pg-role-test1",
					Namespace: f.Namespace(),
				},
				Spec: api.PostgresRoleSpec{
					Provider: &api.ProviderSpec{
						Vault: &api.VaultSpec{
							Address:             f.VaultUrl,
							TokenSecret:         framework.VaultTokenSecret,
							SkipTLSVerification: true,
						},
					},
					Database: &api.DatabaseConfigForPostgres{
						Name:             "postgres-test1",
						CredentialSecret: framework.PostgresCredentialSecret,
						ConnectionUrl:    fmt.Sprintf("postgresql://{{username}}:{{password}}@%s/postgres?sslmode=disable", f.PostgresUrl),
						AllowedRoles:     "*",
					},
					DBName: "postgres-test1",
					CreationStatements: []string{
						"CREATE ROLE \"{{name}}\" WITH LOGIN PASSWORD '{{password}}' VALID UNTIL '{{expiration}}';",
						"GRANT SELECT ON ALL TABLES IN SCHEMA public TO \"{{name}}\";",
					},
					MaxTTL:     "1h",
					DefaultTTL: "300",
				},
			}

			_, err := f.DBClient.AuthorizationV1alpha1().PostgresRoles(pgRole.Namespace).Create(&pgRole)
			Expect(err).NotTo(HaveOccurred(), "Create PostgresRole")

			IsPostgresRoleCreated(pgRole.Name, pgRole.Namespace)
		})

		AfterEach(func() {
			err := f.DBClient.AuthorizationV1alpha1().PostgresRoles(pgRole.Namespace).Delete(pgRole.Name, &metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred(), "Delete PostgresRole")

			IsPostgresRoleDeleted(pgRole.Name, pgRole.Namespace)
			IsVaultDatabaseRoleDeleted(pgRole.Name)
		})

		Context("Create", func() {
			var pgRoleBinding *api.PostgresRoleBinding
			BeforeEach(func() {
				pgRoleBinding = &api.PostgresRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pg-read",
						Namespace: f.Namespace(),
					},
					Spec: api.PostgresRoleBindingSpec{
						RoleRef: pgRole.Name,
						Subjects: []rbacv1.Subject{
							{
								Name:      "pg-sa",
								Kind:      rbacv1.ServiceAccountKind,
								Namespace: f.Namespace(),
							},
						},
						Store: api.Store{
							Secret: "pg-cred",
						},
					},
				}
			})

			AfterEach(func() {
				err := f.DBClient.AuthorizationV1alpha1().PostgresRoleBindings(pgRoleBinding.Namespace).Delete(pgRoleBinding.Name, &metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred(), "Delete PostgresRoleBindings")

				IsPostgresRoleBindingDeleted(pgRoleBinding.Name, pgRoleBinding.Namespace)

				IsSecretDeleted(pgRoleBinding.Spec.Store.Secret, pgRoleBinding.Namespace)
				IsRbacRoleDeleted(fmt.Sprintf("postgresrolebinding-%s-credential-reader", pgRoleBinding.Name), pgRoleBinding.Namespace)
				IsRbacRoleBindingDeleted(fmt.Sprintf("postgresrolebinding-%s-credential-reader", pgRoleBinding.Name), pgRoleBinding.Namespace)
			})

			It("should be successful", func() {
				_, err := f.DBClient.AuthorizationV1alpha1().PostgresRoleBindings(pgRoleBinding.Namespace).Create(pgRoleBinding)
				Expect(err).NotTo(HaveOccurred(), "Create PostgresRoleBinding")

				IsPostgresRoleBindingCreated(pgRoleBinding.Name, pgRoleBinding.Namespace)

				IsSecretCreated(pgRoleBinding.Spec.Store.Secret, pgRoleBinding.Namespace)

				IsSecretCreated(pgRoleBinding.Spec.Store.Secret, pgRoleBinding.Namespace)
				IsRbacRoleCreated(fmt.Sprintf("postgresrolebinding-%s-credential-reader", pgRoleBinding.Name), pgRoleBinding.Namespace)
				IsRbacRoleBindingCreated(fmt.Sprintf("postgresrolebinding-%s-credential-reader", pgRoleBinding.Name), pgRoleBinding.Namespace)
			})
		})
	})
})
