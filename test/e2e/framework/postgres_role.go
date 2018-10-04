package framework

import (
	api "github.com/kubedb/apimachinery/apis/authorization/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (f *Framework) CreatePostgresRole(p *api.PostgresRole) error {
	_, err := f.DBClient.AuthorizationV1alpha1().PostgresRoles(f.namespace).Create(p)
	return err
}

func (f *Framework) DeletePostgresRole(name, namespace string) error {
	return f.DBClient.AuthorizationV1alpha1().PostgresRoles(f.namespace).Delete(name, deleteInForeground())
}

func (f *Framework) GetPostgresRole(name, namespace string) (*api.PostgresRole, error) {
	return f.DBClient.AuthorizationV1alpha1().PostgresRoles(f.namespace).Get(name, metav1.GetOptions{})
}
