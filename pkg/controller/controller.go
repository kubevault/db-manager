package controller

import (
	"fmt"

	crdutils "github.com/appscode/kutil/apiextensions/v1beta1"
	"github.com/appscode/kutil/tools/queue"
	"github.com/golang/glog"
	api "github.com/kubedb/apimachinery/apis/authorization/v1alpha1"
	cs "github.com/kubedb/apimachinery/client/clientset/versioned"
	dbinformers "github.com/kubedb/apimachinery/client/informers/externalversions"
	dblisters "github.com/kubedb/apimachinery/client/listers/authorization/v1alpha1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	crd_cs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	appcat "kmodules.xyz/custom-resources/apis/appcatalog/v1alpha1"
	appcat_cs "kmodules.xyz/custom-resources/client/clientset/versioned"
	appcatinformers "kmodules.xyz/custom-resources/client/informers/externalversions"
	appcatlisters "kmodules.xyz/custom-resources/client/listers/appcatalog/v1alpha1"
)

type Controller struct {
	config

	kubeClient    kubernetes.Interface
	dbClient      cs.Interface
	crdClient     crd_cs.ApiextensionsV1beta1Interface
	catalogClient appcat_cs.Interface
	recorder      record.EventRecorder

	kubeInformerFactory   informers.SharedInformerFactory
	dbInformerFactory     dbinformers.SharedInformerFactory
	appcatInformerFactory appcatinformers.SharedInformerFactory

	// PostgresRole
	pgRoleQueue    *queue.Worker
	pgRoleInformer cache.SharedIndexInformer
	pgRoleLister   dblisters.PostgresRoleLister

	// MySQLRole
	myRoleQueue    *queue.Worker
	myRoleInformer cache.SharedIndexInformer
	myRoleLister   dblisters.MySQLRoleLister

	// MongoDBRole
	mgRoleQueue    *queue.Worker
	mgRoleInformer cache.SharedIndexInformer
	mgRoleLister   dblisters.MongoDBRoleLister

	// DatabaseAccessRequest
	dbAccessQueue    *queue.Worker
	dbAccessInformer cache.SharedIndexInformer
	dbAccessLister   dblisters.DatabaseAccessRequestLister

	// AppBinding
	appBindingInformer cache.SharedIndexInformer
	appBindingLister   appcatlisters.AppBindingLister

	// Contain the currently processing finalizer
	processingFinalizer map[string]bool
}

func (c *Controller) ensureCustomResourceDefinitions() error {
	crds := []*apiextensions.CustomResourceDefinition{
		api.PostgresRole{}.CustomResourceDefinition(),
		api.MySQLRole{}.CustomResourceDefinition(),
		api.MongoDBRole{}.CustomResourceDefinition(),
		api.DatabaseAccessRequest{}.CustomResourceDefinition(),
		appcat.AppBinding{}.CustomResourceDefinition(),
	}
	return crdutils.RegisterCRDs(c.crdClient, crds)
}

func (c *Controller) RunInformers(stopCh <-chan struct{}) {
	defer runtime.HandleCrash()

	glog.Info("Starting KubeDB user manager controller")

	// c.kubeInformerFactory.Start(stopCh)
	c.dbInformerFactory.Start(stopCh)

	// Wait for all involved caches to be synced, before processing items from the queue is started
	//for _, v := range c.kubeInformerFactory.WaitForCacheSync(stopCh) {
	//	if !v {
	//		runtime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
	//		return
	//	}
	//}
	for _, v := range c.dbInformerFactory.WaitForCacheSync(stopCh) {
		if !v {
			runtime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
			return
		}
	}

	go c.pgRoleQueue.Run(stopCh)
	go c.myRoleQueue.Run(stopCh)
	go c.mgRoleQueue.Run(stopCh)

	go c.dbAccessQueue.Run(stopCh)

	<-stopCh
	glog.Info("Stopping KubeDB user manager controller")
}
