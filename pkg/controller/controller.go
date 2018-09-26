package controller

import (
	"fmt"

	crdutils "github.com/appscode/kutil/apiextensions/v1beta1"
	"github.com/appscode/kutil/tools/queue"
	"github.com/golang/glog"
	api "github.com/kubedb/user-manager/apis/authorization/v1alpha1"
	cs "github.com/kubedb/user-manager/client/clientset/versioned"
	dbinformers "github.com/kubedb/user-manager/client/informers/externalversions"
	dblisters "github.com/kubedb/user-manager/client/listers/authorization/v1alpha1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	crd_cs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
)

type UserManagerController struct {
	config

	kubeClient kubernetes.Interface
	dbClient   cs.Interface
	crdClient  crd_cs.ApiextensionsV1beta1Interface
	recorder   record.EventRecorder

	kubeInformerFactory informers.SharedInformerFactory
	dbInformerFactory   dbinformers.SharedInformerFactory

	// PostgresRole
	pgRoleQueue    *queue.Worker
	pgRoleInformer cache.SharedIndexInformer
	pgRoleLister   dblisters.PostgresRoleLister

	// PostgresRoleBinding
	pgRoleBindingQueue    *queue.Worker
	pgRoleBindingInformer cache.SharedIndexInformer
	pgRoleBindingLister   dblisters.PostgresRoleBindingLister

	// MySQLRole
	myRoleQueue    *queue.Worker
	myRoleInformer cache.SharedIndexInformer
	myRoleLister   dblisters.MySQLRoleLister

	// MySQLRoleBinding
	myRoleBindingQueue    *queue.Worker
	myRoleBindingInformer cache.SharedIndexInformer
	myRoleBindingLister   dblisters.MySQLRoleBindingLister

	// MongoDBRole
	mgRoleQueue    *queue.Worker
	mgRoleInformer cache.SharedIndexInformer
	mgRoleLister   dblisters.MongoDBRoleLister

	// MongoDBRoleBinding
	mgRoleBindingQueue    *queue.Worker
	mgRoleBindingInformer cache.SharedIndexInformer
	mgRoleBindingLister   dblisters.MongoDBRoleBindingLister

	// Contain the currently processing finalizer
	processingFinalizer map[string]bool
}

func (c *UserManagerController) ensureCustomResourceDefinitions() error {
	crds := []*apiextensions.CustomResourceDefinition{
		api.PostgresRole{}.CustomResourceDefinition(),
		api.PostgresRoleBinding{}.CustomResourceDefinition(),
		api.MySQLRole{}.CustomResourceDefinition(),
		api.MySQLRoleBinding{}.CustomResourceDefinition(),
		api.MongoDBRole{}.CustomResourceDefinition(),
		api.MongoDBRoleBinding{}.CustomResourceDefinition(),
	}
	return crdutils.RegisterCRDs(c.crdClient, crds)
}

func (c *UserManagerController) RunInformers(stopCh <-chan struct{}) {
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
	go c.pgRoleBindingQueue.Run(stopCh)

	go c.myRoleQueue.Run(stopCh)
	go c.myRoleBindingQueue.Run(stopCh)

	go c.mgRoleQueue.Run(stopCh)
	go c.mgRoleBindingQueue.Run(stopCh)

	go c.LeaseRenewer(c.LeaseRenewTime)

	<-stopCh
	glog.Info("Stopping KubeDB user manager controller")
}
