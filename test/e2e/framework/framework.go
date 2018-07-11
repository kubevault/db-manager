package framework

import (
	"github.com/appscode/go/crypto/rand"
	cs "github.com/kubedb/user-manager/client/clientset/versioned"
	crd_cs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ka "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset"
)

type Framework struct {
	KubeClient      kubernetes.Interface
	MessengerClient cs.Interface
	CRDClient       crd_cs.ApiextensionsV1beta1Interface
	KAClient        ka.Interface

	namespace      string
	WebhookEnabled bool

	ClientConfig *rest.Config
}

func New(
	kubeClient kubernetes.Interface,
	messengerClient cs.Interface,
	crdClient crd_cs.ApiextensionsV1beta1Interface,
	kaClient ka.Interface,
	webhookEnabled bool,
	clientConfig *rest.Config) *Framework {

	return &Framework{
		KubeClient:      kubeClient,
		MessengerClient: messengerClient,
		CRDClient:       crdClient,
		KAClient:        kaClient,

		namespace:      rand.WithUniqSuffix("messenger-e2e"),
		WebhookEnabled: webhookEnabled,

		ClientConfig: clientConfig,
	}
}

func (f *Framework) Invoke() *Invocation {
	return &Invocation{
		Framework: f,
		app:       rand.WithUniqSuffix("test-messenger"),
	}
}

type Invocation struct {
	*Framework
	app string
}

func (f *Invocation) App() string {
	return f.app
}
