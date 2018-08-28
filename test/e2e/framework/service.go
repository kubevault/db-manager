package framework

import (
	corev1 "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (f *Framework) CreateService(obj corev1.Service) error {
	_, err := f.KubeClient.CoreV1().Services(obj.Namespace).Create(&obj)
	return err
}

func (f *Framework) DeleteService(name, namespace string) error {
	_, err := f.KubeClient.CoreV1().Services(namespace).Get(name, metav1.GetOptions{})
	if kerr.IsNotFound(err) {
		return nil
	}
	return f.KubeClient.CoreV1().Services(namespace).Delete(name, deleteInForeground())
}
