package e2e_test

import (
	"testing"
	"time"

	logs "github.com/appscode/go/log/golog"
	"github.com/appscode/kutil/meta"
	"github.com/appscode/kutil/tools/clientcmd"
	cs "github.com/kubedb/user-manager/client/clientset/versioned"
	"github.com/kubedb/user-manager/client/clientset/versioned/scheme"
	"github.com/kubedb/user-manager/test/e2e/framework"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	crd_cs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	ka "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
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
	scheme.AddToScheme(clientsetscheme.Scheme)
	scheme.AddToScheme(legacyscheme.Scheme)

	clientConfig, err := clientcmd.BuildConfigFromContext(options.KubeConfig, options.KubeContext)
	Expect(err).NotTo(HaveOccurred())

	kubeClient, err := kubernetes.NewForConfig(clientConfig)
	Expect(err).NotTo(HaveOccurred())

	messengerClient, err := cs.NewForConfig(clientConfig)
	Expect(err).NotTo(HaveOccurred())

	crdClient, err := crd_cs.NewForConfig(clientConfig)
	Expect(err).NotTo(HaveOccurred())

	kaClient, err := ka.NewForConfig(clientConfig)
	Expect(err).NotTo(HaveOccurred())

	root = framework.New(kubeClient, messengerClient, crdClient, kaClient, options.StartAPIServer, clientConfig)
	err = root.CreateNamespace()
	Expect(err).NotTo(HaveOccurred())
	By("Using test namespace " + root.Namespace() + "...")

	go root.StartAPIServerAndOperator(options.KubeConfig, options.ExtraOptions)
	root.EventuallyAPIServerReady("v1alpha1.admission.users.kubedb.com").Should(Succeed())
	// let's API server be warmed up
	time.Sleep(time.Second * 5)
})

var _ = AfterSuite(func() {
	if options.StartAPIServer {
		By("Cleaning API server and Webhook stuff")
		root.KubeClient.AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Delete("admission.users.kubedb.com", meta.DeleteInBackground())
		root.KubeClient.CoreV1().Endpoints(root.Namespace()).Delete("messenger-local-apiserver", meta.DeleteInBackground())
		root.KubeClient.CoreV1().Services(root.Namespace()).Delete("messenger-local-apiserver", meta.DeleteInBackground())
		root.KAClient.ApiregistrationV1beta1().APIServices().Delete("v1alpha1.admission.users.kubedb.com", meta.DeleteInBackground())
		root.KAClient.ApiregistrationV1beta1().APIServices().Delete("v1alpha1.users.kubedb.com", meta.DeleteInBackground())
	}

	By("Removing CRD group...")
	crds, err := root.CRDClient.CustomResourceDefinitions().List(metav1.ListOptions{
		LabelSelector: labels.Set{
			"app": "messenger",
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
