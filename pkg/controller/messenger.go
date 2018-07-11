package controller

import (
	"fmt"
	"strings"
	"time"

	"github.com/appscode/envconfig"
	"github.com/appscode/go-notify"
	"github.com/appscode/go-notify/unified"
	"github.com/appscode/kubernetes-webhook-util/admission"
	hooks "github.com/appscode/kubernetes-webhook-util/admission/v1beta1"
	webhook "github.com/appscode/kubernetes-webhook-util/admission/v1beta1/generic"
	"github.com/appscode/kutil/tools/queue"
	"github.com/golang/glog"
	"github.com/kubedb/user-manager/apis/authorization"
	api "github.com/kubedb/user-manager/apis/authorization/v1alpha1"
	"github.com/kubedb/user-manager/client/clientset/versioned/typed/authorization/v1alpha1/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func (c *MessengerController) NewNotifierWebhook() hooks.AdmissionHook {
	return webhook.NewGenericWebhook(
		schema.GroupVersionResource{
			Group:    "admission.authorization.kubedb.com",
			Version:  "v1alpha1",
			Resource: api.ResourceMessages,
		},
		api.ResourceMessage,
		[]string{authorization.GroupName},
		api.SchemeGroupVersion.WithKind(api.ResourceKindMessage),
		nil,
		&admission.ResourceHandlerFuncs{
			CreateFunc: func(obj runtime.Object) (runtime.Object, error) {
				return nil, obj.(*api.Message).IsValid()
			},
			UpdateFunc: func(oldObj, newObj runtime.Object) (runtime.Object, error) {
				return nil, newObj.(*api.Message).IsValid()
			},
		},
	)
}
func (c *MessengerController) initMessageWatcher() {
	c.messageInformer = c.authzInformerFactory.Authorization().V1alpha1().Messages().Informer()
	c.messageQueue = queue.New(api.ResourceKindMessage, c.MaxNumRequeues, c.NumThreads, c.reconcileMessage)
	//c.messageInformer.AddEventHandler(queue.DefaultEventHandler(c.messageQueue.GetQueue()))
	c.messageInformer.AddEventHandler(queue.NewEventHandler(c.messageQueue.GetQueue(), c.enqueueUpdate))
	c.messageLister = c.authzInformerFactory.Authorization().V1alpha1().Messages().Lister()
}

func (c *MessengerController) reconcileMessage(key string) error {
	obj, exist, err := c.messageInformer.GetIndexer().GetByKey(key)
	if err != nil {
		glog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exist {
		glog.Warningf("Message %s does not exist anymore\n", key)
	} else {
		glog.Infof("Sync/Add/Update for Message %s\n", key)

		msg := obj.(*api.Message)
		fmt.Println("Message name", msg.Name)
		glog.Infoln("Message is sending...")

		msgStatus := &api.MessageStatus{}
		err := c.send(msg)
		if err != nil {
			msgStatus.ErrorMessage = fmt.Sprintf("Sending Message with key %s failed: %v", key, err)
			glog.Errorf(msgStatus.ErrorMessage)
		} else {
			msgStatus.SentTimestamp = &metav1.Time{time.Now()}
			glog.Infof("Message with key %s has been sent", key)
		}

		_, updateErr := util.UpdateMessageStatus(c.authzClient.AuthorizationV1alpha1(), msg, func(in *api.MessageStatus) *api.MessageStatus {
			*in = *msgStatus
			return in
		}, api.EnableStatusSubresource)
		if updateErr != nil {
			//glog.Errorf("Failed to update status for Message with key %s: %v", key, updateErr)
			return fmt.Errorf("Failed to update status for Message with key %s: %v", key, updateErr)
		}
		if msgStatus.ErrorMessage != "" {
			return fmt.Errorf("%s", msgStatus.ErrorMessage)
		}
	}
	return nil
}

func (c *MessengerController) deleteMessengerNotifier(repository *api.Message) error {
	return nil
}

func (c *MessengerController) send(msg *api.Message) error {
	messagingService, err := c.authzClient.AuthorizationV1alpha1().MessagingServices(msg.Namespace).Get(msg.Spec.Service, metav1.GetOptions{})
	if err != nil {
		return err
	}

	cred, err := c.getLoader(messagingService.Spec.CredentialSecretName, messagingService.Namespace)
	if err != nil {
		return err
	}

	notifier, err := unified.LoadVia(strings.ToLower(messagingService.Spec.Drive), cred)
	if err != nil {
		return err
	}

	switch n := notifier.(type) {
	case notify.ByEmail:
		return n.To(messagingService.Spec.To[0], messagingService.Spec.To[1:]...).
			WithSubject(msg.Spec.Email).
			WithBody(msg.Spec.Message).
			WithNoTracking().
			Send()
	case notify.BySMS:
		return n.To(messagingService.Spec.To[0], messagingService.Spec.To[1:]...).
			WithBody(msg.Spec.Sms).
			Send()
	case notify.ByChat:
		return n.To(messagingService.Spec.To[0], messagingService.Spec.To[1:]...).
			WithBody(msg.Spec.Chat).
			Send()
	case notify.ByPush:
		return n.To(messagingService.Spec.To...).
			WithBody(msg.Spec.Chat).
			Send()
	}

	return nil
}

func (c *MessengerController) getLoader(credentialSecretName, namespace string) (envconfig.LoaderFunc, error) {
	if credentialSecretName == "" {
		return func(key string) (string, bool) {
			return "", false
		}, nil
	}
	cfg, err := c.kubeClient.CoreV1().
		Secrets(namespace).
		Get(credentialSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return func(key string) (value string, found bool) {
		var bytes []byte
		bytes, found = cfg.Data[key]
		value = string(bytes)
		return
	}, nil
}
