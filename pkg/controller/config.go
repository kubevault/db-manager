package controller

import (
	"time"

	cs "github.com/kubedb/user-manager/client/clientset/versioned"
	authzinformers "github.com/kubedb/user-manager/client/informers/externalversions"
	"github.com/kubedb/user-manager/pkg/eventer"
	core "k8s.io/api/core/v1"
	crd_cs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type config struct {
	MessengerImageTag  string
	DockerRegistry     string
	MaxNumRequeues     int
	NumThreads         int
	ResyncPeriod       time.Duration
	GarbageCollectTime time.Duration
}

type Config struct {
	config

	ClientConfig    *rest.Config
	KubeClient      kubernetes.Interface
	MessengerClient cs.Interface
	CRDClient       crd_cs.ApiextensionsV1beta1Interface
}

func NewConfig(clientConfig *rest.Config) *Config {
	return &Config{
		ClientConfig: clientConfig,
	}
}

func (c *Config) New() (*MessengerController, error) {
	tweakListOptions := func(opt *metav1.ListOptions) {
		opt.IncludeUninitialized = true
	}
	ctrl := &MessengerController{
		config:               c.config,
		kubeClient:           c.KubeClient,
		authzClient:          c.MessengerClient,
		crdClient:            c.CRDClient,
		kubeInformerFactory:  informers.NewFilteredSharedInformerFactory(c.KubeClient, c.ResyncPeriod, core.NamespaceAll, tweakListOptions),
		authzInformerFactory: authzinformers.NewSharedInformerFactory(c.MessengerClient, c.ResyncPeriod),
		recorder:             eventer.NewEventRecorder(c.KubeClient, "messenger-controller"),
	}

	if err := ctrl.ensureCustomResourceDefinitions(); err != nil {
		return nil, err
	}

	ctrl.initMessageWatcher()

	return ctrl, nil
}
