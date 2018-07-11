package v1alpha1

import (
	crdutils "github.com/appscode/kutil/apiextensions/v1beta1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
)

var (
	EnableStatusSubresource bool
)

func (p MessagingService) CustomResourceDefinition() *apiextensions.CustomResourceDefinition {
	return crdutils.NewCustomResourceDefinition(crdutils.Config{
		Group:         SchemeGroupVersion.Group,
		Plural:        ResourceMessagingServices,
		Singular:      ResourceMessagingService,
		Kind:          ResourceKindMessagingService,
		ResourceScope: string(apiextensions.NamespaceScoped),
		Versions: []apiextensions.CustomResourceDefinitionVersion{
			{
				Name:    SchemeGroupVersion.Version,
				Served:  true,
				Storage: true,
			},
		},
		Labels: crdutils.Labels{
			LabelsMap: map[string]string{"app": "messenger"},
		},
		SpecDefinitionName:      "github.com/kubedb/user-manager/apis/authorization/v1alpha1.MessagingService",
		EnableValidation:        true,
		GetOpenAPIDefinitions:   GetOpenAPIDefinitions,
		EnableStatusSubresource: EnableStatusSubresource,
	})
}

func (p MessagingService) IsValid() error {
	return nil
}

func (p Message) CustomResourceDefinition() *apiextensions.CustomResourceDefinition {
	return crdutils.NewCustomResourceDefinition(crdutils.Config{
		Group:         SchemeGroupVersion.Group,
		Plural:        ResourceMessages,
		Singular:      ResourceMessage,
		Kind:          ResourceKindMessage,
		ResourceScope: string(apiextensions.NamespaceScoped),
		Versions: []apiextensions.CustomResourceDefinitionVersion{
			{
				Name:    SchemeGroupVersion.Version,
				Served:  true,
				Storage: true,
			},
		},
		Labels: crdutils.Labels{
			LabelsMap: map[string]string{"app": "messenger"},
		},
		SpecDefinitionName:      "github.com/kubedb/user-manager/apis/authorization/v1alpha1.Message",
		EnableValidation:        true,
		GetOpenAPIDefinitions:   GetOpenAPIDefinitions,
		EnableStatusSubresource: EnableStatusSubresource,
	})
}

func (p Message) IsValid() error {
	return nil
}
