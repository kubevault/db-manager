package e2e_test

import (
	"fmt"
	"time"

	api "github.com/kubedb/user-manager/apis/authorization/v1alpha1"
	"github.com/kubedb/user-manager/test/e2e/framework"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	kerrors "k8s.io/apimachinery/pkg/api/errors"

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

})
