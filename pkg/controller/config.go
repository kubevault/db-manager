package controller

import (
	"time"

	"github.com/appscode/go/log/golog"
	cs "github.com/kubedb/apimachinery/client/clientset/versioned"
	dbinformers "github.com/kubedb/apimachinery/client/informers/externalversions"
	"github.com/kubevault/db-manager/pkg/eventer"
	core "k8s.io/api/core/v1"
	crd_cs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	appcat_cs "kmodules.xyz/custom-resources/client/clientset/versioned"
	appcatinformers "kmodules.xyz/custom-resources/client/informers/externalversions"
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

	ClientConfig  *rest.Config
	KubeClient    kubernetes.Interface
	DbClient      cs.Interface
	CRDClient     crd_cs.ApiextensionsV1beta1Interface
	CatalogClient appcat_cs.Interface
}

func NewConfig(clientConfig *rest.Config) *Config {
	return &Config{
		ClientConfig: clientConfig,
	}
}

func (c *Config) New() (*Controller, error) {
	tweakListOptions := func(opt *metav1.ListOptions) {
		opt.IncludeUninitialized = true
	}
	ctrl := &Controller{
		config:                c.config,
		kubeClient:            c.KubeClient,
		dbClient:              c.DbClient,
		crdClient:             c.CRDClient,
		catalogClient:         c.CatalogClient,
		kubeInformerFactory:   informers.NewFilteredSharedInformerFactory(c.KubeClient, c.ResyncPeriod, core.NamespaceAll, tweakListOptions),
		dbInformerFactory:     dbinformers.NewSharedInformerFactory(c.DbClient, c.ResyncPeriod),
		appcatInformerFactory: appcatinformers.NewSharedInformerFactory(c.CatalogClient, c.ResyncPeriod),
		recorder:              eventer.NewEventRecorder(c.KubeClient, "user-manager-controller"),
		processingFinalizer:   map[string]bool{},
	}

	if err := ctrl.ensureCustomResourceDefinitions(); err != nil {
		return nil, err
	}

	ctrl.initPostgresRoleWatcher()
	ctrl.initMySQLRoleWatcher()
	ctrl.initMongoDBRoleWatcher()
	ctrl.initDatabaseAccessWatcher()

	return ctrl, nil
}
