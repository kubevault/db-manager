package framework

import (
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
)

func (f *Framework) CreateSecret(obj *core.Secret) (*core.Secret, error) {
	return f.KubeClient.CoreV1().Secrets(obj.Namespace).Create(obj)
}

func (f *Framework) DeleteSecret(name, namespace string) error {
	_, err := f.KubeClient.CoreV1().Secrets(namespace).Get(name, metav1.GetOptions{})
	if kerr.IsNotFound(err) {
		return nil
	}
	return f.KubeClient.CoreV1().Secrets(namespace).Delete(name, deleteInForeground())
}
