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
	postgresRoleQueue    *queue.Worker
	postgresRoleInformer cache.SharedIndexInformer
	postgresRoleLister   dblisters.PostgresRoleLister

	// PostgresRoleBinding
	postgresRoleBindingQueue    *queue.Worker
	postgresRoleBindingInformer cache.SharedIndexInformer
	postgresRoleBindingLister   dblisters.PostgresRoleBindingLister

	// MysqlRole
	mysqlRoleQueue    *queue.Worker
	mysqlRoleInformer cache.SharedIndexInformer
	mysqlRoleLister   dblisters.MysqlRoleLister

	// MysqlRoleBinding
	mysqlRoleBindingQueue    *queue.Worker
	mysqlRoleBindingInformer cache.SharedIndexInformer
	mysqlRoleBindingLister   dblisters.MysqlRoleBindingLister

	// MongodbRole
	mongodbRoleQueue    *queue.Worker
	mongodbRoleInformer cache.SharedIndexInformer
	mongodbRoleLister   dblisters.MongodbRoleLister

	// MongodbRoleBinding
	mongodbRoleBindingQueue    *queue.Worker
	mongodbRoleBindingInformer cache.SharedIndexInformer
	mongodbRoleBindingLister   dblisters.MongodbRoleBindingLister

	// Contain the currently processing finalizer
	processingFinalizer map[string]bool
}

func (c *UserManagerController) ensureCustomResourceDefinitions() error {
	crds := []*apiextensions.CustomResourceDefinition{
		api.PostgresRole{}.CustomResourceDefinition(),
		api.PostgresRoleBinding{}.CustomResourceDefinition(),
		api.MysqlRole{}.CustomResourceDefinition(),
		api.MysqlRoleBinding{}.CustomResourceDefinition(),
		api.MongodbRole{}.CustomResourceDefinition(),
		api.MongodbRoleBinding{}.CustomResourceDefinition(),
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

	go c.postgresRoleQueue.Run(stopCh)
	go c.postgresRoleBindingQueue.Run(stopCh)

	go c.mysqlRoleQueue.Run(stopCh)
	go c.mysqlRoleBindingQueue.Run(stopCh)

	go c.mongodbRoleQueue.Run(stopCh)
	go c.mongodbRoleBindingQueue.Run(stopCh)

	go c.LeaseRenewer(c.LeaseRenewTime)

	<-stopCh
	glog.Info("Stopping KubeDB user manager controller")
}
