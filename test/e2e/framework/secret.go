package framework

import (
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func (f *Invocation) NewSecret(name, namespace, authTokenToSendMessage string, labels map[string]string) *core.Secret {
	return &core.Secret{
		ObjectMeta: newObjectMeta(name, namespace, labels),
		StringData: map[string]string{
			"HIPCHAT_AUTH_TOKEN": authTokenToSendMessage,
		},
	}
}

func (f *Invocation) CreateSecret(obj *core.Secret) (*core.Secret, error) {
	return f.KubeClient.CoreV1().Secrets(obj.Namespace).Create(obj)
}

func (f *Invocation) DeleteAllSecrets() error {
	secrets, err := f.KubeClient.CoreV1().Secrets(metav1.NamespaceAll).List(metav1.ListOptions{
		LabelSelector: labels.Set{
			"app": f.App(),
		}.String(),
	})
	if err != nil {
		return err
	}

	for _, secret := range secrets.Items {
		err := f.KubeClient.CoreV1().Secrets(secret.Namespace).Delete(secret.Name, &metav1.DeleteOptions{})
		if kerr.IsNotFound(err) {
			err = nil
		}
		if err != nil {
			return err
		}
	}

	return nil
}
