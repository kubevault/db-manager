package framework

import (
	core "k8s.io/api/core/v1"
)

func (f *Framework) CreateSecret(obj *core.Secret) (*core.Secret, error) {
	return f.KubeClient.CoreV1().Secrets(obj.Namespace).Create(obj)
}

func (f *Framework) DeleteSecret(name, namespace string) error {
	return f.KubeClient.CoreV1().Secrets(namespace).Delete(name, deleteInForeground())
}
