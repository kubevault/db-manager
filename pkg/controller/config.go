package controller

import (
	"time"

	"github.com/appscode/go/log/golog"
	cs "github.com/kubedb/user-manager/client/clientset/versioned"
	dbinformers "github.com/kubedb/user-manager/client/informers/externalversions"
	"github.com/kubedb/user-manager/pkg/eventer"
	core "k8s.io/api/core/v1"
	crd_cs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	LoggerOptions golog.Options
)

type config struct {
	UserManagerImageTag string
	DockerRegistry      string
	MaxNumRequeues      int
	NumThreads          int
	ResyncPeriod        time.Duration
	LeaseRenewTime      time.Duration
}

type Config struct {
	config

	ClientConfig *rest.Config
	KubeClient   kubernetes.Interface
	DbClient     cs.Interface
	CRDClient    crd_cs.ApiextensionsV1beta1Interface
}

func NewConfig(clientConfig *rest.Config) *Config {
	return &Config{
		ClientConfig: clientConfig,
	}
}

func (c *Config) New() (*UserManagerController, error) {
	tweakListOptions := func(opt *metav1.ListOptions) {
		opt.IncludeUninitialized = true
	}
	ctrl := &UserManagerController{
		config:              c.config,
		kubeClient:          c.KubeClient,
		dbClient:            c.DbClient,
		crdClient:           c.CRDClient,
		kubeInformerFactory: informers.NewFilteredSharedInformerFactory(c.KubeClient, c.ResyncPeriod, core.NamespaceAll, tweakListOptions),
		dbInformerFactory:   dbinformers.NewSharedInformerFactory(c.DbClient, c.ResyncPeriod),
		recorder:            eventer.NewEventRecorder(c.KubeClient, "user-manager-controller"),
		processingFinalizer: map[string]bool{},
	}

	if err := ctrl.ensureCustomResourceDefinitions(); err != nil {
		return nil, err
	}

	ctrl.initPostgresRoleWatcher()
	ctrl.initPostgresRoleBindingWatcher()

	return ctrl, nil
}
