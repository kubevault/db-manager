package framework

import (
	"encoding/json"
	"fmt"
	api "github.com/kubedb/user-manager/apis/users/v1alpha1"
	. "github.com/onsi/gomega"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"net/http"
	"strings"
	"time"
)

func (f *Invocation) NewMessagingService(
	name, namespace string, labels map[string]string,
	drive, secret string, to []string) *api.MessagingService {
	return &api.MessagingService{
		ObjectMeta: newObjectMeta(name, namespace, labels),
		Spec: api.MessagingServiceSpec{
			Drive:                drive,
			To:                   to,
			CredentialSecretName: secret,
		},
	}
}

func (f *Invocation) NewMessage(
	genName, namespace string, labels map[string]string,
	service, message, chat, email, sms string) *api.Message {
	return &api.Message{
		ObjectMeta: newObjectMetaWithGenerateName(genName, namespace, labels),
		Spec: api.MessageSpec{
			Service: service,
			Message: message,
			Chat:    chat,
			Email:   email,
			Sms:     sms,
		},
	}
}

func (f *Invocation) CreateMessagingService(obj *api.MessagingService) (*api.MessagingService, error) {
	return f.MessengerClient.MessengerV1alpha1().MessagingServices(obj.Namespace).Create(obj)
}

func (f *Invocation) CreateMessage(obj *api.Message) (*api.Message, error) {
	return f.MessengerClient.MessengerV1alpha1().Messages(obj.Namespace).Create(obj)
}

func (f *Invocation) DeleteAllCRDs() error {
	// delete all messagingSerices
	messages, err := f.MessengerClient.MessengerV1alpha1().Messages(f.namespace).List(metav1.ListOptions{
		LabelSelector: labels.Set{
			"app": f.app,
		}.String(),
	})
	if err != nil {
		return err
	}

	for _, message := range messages.Items {
		err := f.MessengerClient.MessengerV1alpha1().Messages(f.namespace).Delete(message.Name, &metav1.DeleteOptions{})
		if kerr.IsNotFound(err) {
			err = nil
		}
		if err != nil {
			return err
		}
	}

	// delete all messagingSerices
	messagingServices, err := f.MessengerClient.MessengerV1alpha1().MessagingServices(f.namespace).List(metav1.ListOptions{
		LabelSelector: labels.Set{
			"app": f.app,
		}.String(),
	})
	if err != nil {
		return err
	}

	for _, messagingService := range messagingServices.Items {
		err := f.MessengerClient.MessengerV1alpha1().MessagingServices(f.namespace).Delete(messagingService.Name, &metav1.DeleteOptions{})
		if kerr.IsNotFound(err) {
			err = nil
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func (f *Invocation) EventuallyCheckMessage(msgBoby, timeBeforeSend, authTokenToSeeHistory string) GomegaAsyncAssertion {
	type msg struct {
		Type    string
		Message string
	}

	var (
		client = http.Client{
			Timeout: time.Minute * 5,
		}
		url  = "http://api.hipchat.com/v2/room/1214663/history?end-date=" + timeBeforeSend
		req  *http.Request
		resp *http.Response
		err  error
		msgs struct {
			Items []msg
		}
		found bool
	)

	return Eventually(
		func() error {
			if req, err = http.NewRequest("GET", url, nil); err != nil {
				return err
			}
			req.Header.Set("Authorization", "Bearer "+authTokenToSeeHistory)

			if resp, err = client.Do(req); err != nil {
				return err
			}

			defer resp.Body.Close()

			if err = json.NewDecoder(resp.Body).Decode(&msgs); err != nil {
				return err
			}

			found = false
			for i := 0; !found && i < len(msgs.Items); i++ {
				if msgs.Items[i].Type == "notification" && msgs.Items[i].Message != "" {
					found = found || strings.Contains(msgs.Items[i].Message, msgBoby)
				}
			}
			if !found {
				return fmt.Errorf("Not found message \"%s\"", msgBoby)
			}

			return nil
		},
		time.Minute*2,
		time.Millisecond*5,
	)
}
