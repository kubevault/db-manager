package controller

import (
	"fmt"

	crdutils "github.com/appscode/kutil/apiextensions/v1beta1"
	"github.com/appscode/kutil/tools/queue"
	"github.com/golang/glog"
	api "github.com/kubedb/user-manager/apis/authorization/v1alpha1"
	cs "github.com/kubedb/user-manager/client/clientset/versioned"
	authzinformers "github.com/kubedb/user-manager/client/informers/externalversions"
	authz_listers "github.com/kubedb/user-manager/client/listers/authorization/v1alpha1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	crd_cs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
)

type MessengerController struct {
	config

	kubeClient  kubernetes.Interface
	authzClient cs.Interface
	crdClient   crd_cs.ApiextensionsV1beta1Interface
	recorder    record.EventRecorder

	kubeInformerFactory  informers.SharedInformerFactory
	authzInformerFactory authzinformers.SharedInformerFactory

	// Notification
	messageQueue    *queue.Worker
	messageInformer cache.SharedIndexInformer
	messageLister   authz_listers.MessageLister
}

func (c *MessengerController) ensureCustomResourceDefinitions() error {
	crds := []*apiextensions.CustomResourceDefinition{
		api.MessagingService{}.CustomResourceDefinition(),
		api.Message{}.CustomResourceDefinition(),
	}
	return crdutils.RegisterCRDs(c.crdClient, crds)
}

func (c *MessengerController) RunInformers(stopCh <-chan struct{}) {
	defer runtime.HandleCrash()

	glog.Info("Starting KubeDB user manager controller")
	c.kubeInformerFactory.Start(stopCh)
	c.authzInformerFactory.Start(stopCh)

	// Wait for all involved caches to be synced, before processing items from the queue is started
	for _, v := range c.kubeInformerFactory.WaitForCacheSync(stopCh) {
		if !v {
			runtime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
			return
		}
	}
	for _, v := range c.authzInformerFactory.WaitForCacheSync(stopCh) {
		if !v {
			runtime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
			return
		}
	}

	go c.garbageCollect(stopCh, c.GarbageCollectTime)
	c.messageQueue.Run(stopCh)
}
