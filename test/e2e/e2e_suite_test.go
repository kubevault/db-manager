package e2e_test

import (
	"testing"
	"time"

	logs "github.com/appscode/go/log/golog"
	"github.com/appscode/kutil/meta"
	"github.com/appscode/kutil/tools/clientcmd"
	"github.com/kubevault/db-manager/pkg/controller"
	"github.com/kubevault/db-manager/test/e2e/framework"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	TIMEOUT = 20 * time.Minute
)

var (
	root *framework.Framework
)

func TestE2e(t *testing.T) {
	logs.InitLogs()
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(TIMEOUT)
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "e2e Suite", []Reporter{junitReporter})
}

var _ = BeforeSuite(func() {
	clientConfig, err := clientcmd.BuildConfigFromContext(options.KubeConfig, options.KubeContext)
	Expect(err).NotTo(HaveOccurred())

	ctrlConfig := controller.NewConfig(clientConfig)
	ctrlConfig.MaxNumRequeues = 5
	ctrlConfig.NumThreads = 1
	ctrlConfig.ResyncPeriod = 10 * time.Minute
	ctrlConfig.LeaseRenewTime = 10 * time.Minute

	err = options.ApplyTo(ctrlConfig)
	Expect(err).NotTo(HaveOccurred())

	ctrl, err := ctrlConfig.New()
	Expect(err).NotTo(HaveOccurred())

	root = framework.New(ctrlConfig.KubeClient, ctrlConfig.DbClient, ctrlConfig.CRDClient, nil, options.StartAPIServer, clientConfig)

	err = root.CreateNamespace()
	Expect(err).NotTo(HaveOccurred())
	By("Using test namespace " + root.Namespace() + "...")

	By("Deploying postgres, mysql, mongodb, vault...")
	err = root.InitialSetup()
	Expect(err).NotTo(HaveOccurred())

	if options.StartAPIServer {
		go root.StartAPIServerAndOperator(options.KubeConfig, options.ExtraOptions)
		root.EventuallyAPIServerReady("v1alpha1.admission.authorization.kubedb.com").Should(Succeed())
		// let's API server be warmed up
		time.Sleep(time.Second * 5)
	} else {
		go ctrl.RunInformers(nil)
	}
})

var _ = AfterSuite(func() {
	if options.StartAPIServer {
		By("Cleaning API server and Webhook stuff")
		root.KubeClient.AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Delete("admission.authorization.kubedb.com", meta.DeleteInBackground())
		root.KubeClient.CoreV1().Endpoints(root.Namespace()).Delete("messenger-local-apiserver", meta.DeleteInBackground())
		root.KubeClient.CoreV1().Services(root.Namespace()).Delete("messenger-local-apiserver", meta.DeleteInBackground())
		root.KAClient.ApiregistrationV1beta1().APIServices().Delete("v1alpha1.admission.authorization.kubedb.com", meta.DeleteInBackground())
		root.KAClient.ApiregistrationV1beta1().APIServices().Delete("v1alpha1.authorization.kubedb.com", meta.DeleteInBackground())
	}

	Expect(root.Cleanup()).NotTo(HaveOccurred())

	By("Removing CRD group...")
	crds, err := root.CRDClient.CustomResourceDefinitions().List(metav1.ListOptions{
		LabelSelector: labels.Set{
			"app": "user-manager",
		}.String(),
	})
	Expect(err).NotTo(HaveOccurred())
	for _, crd := range crds.Items {
		err := root.CRDClient.CustomResourceDefinitions().Delete(crd.Name, &metav1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())
	}

	By("Deleting Namespace...")
	root.DeleteNamespace()
})
