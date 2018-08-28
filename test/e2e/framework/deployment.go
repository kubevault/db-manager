package framework

import (
	. "github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1beta1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (f *Framework) CreateDeployment(obj apps.Deployment) (*apps.Deployment, error) {
	return f.KubeClient.AppsV1beta1().Deployments(obj.Namespace).Create(&obj)
}

func (f *Framework) DeleteDeployment(name, namespace string) error {
	_, err := f.KubeClient.AppsV1().Deployments(namespace).Get(name, metav1.GetOptions{})
	if kerr.IsNotFound(err) {
		return nil
	}
	return f.KubeClient.AppsV1beta1().Deployments(namespace).Delete(name, deleteInBackground())
}

func (f *Framework) EventuallyDeployment(meta metav1.ObjectMeta) GomegaAsyncAssertion {
	return Eventually(func() *apps.Deployment {
		obj, err := f.KubeClient.AppsV1beta1().Deployments(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		return obj
	})
}
