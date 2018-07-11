/*
Copyright 2018 The Attic Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by lister-gen. DO NOT EDIT.

package v1alpha1

import (
	v1alpha1 "github.com/kubedb/user-manager/apis/users/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// MessageLister helps list Messages.
type MessageLister interface {
	// List lists all Messages in the indexer.
	List(selector labels.Selector) (ret []*v1alpha1.Message, err error)
	// Messages returns an object that can list and get Messages.
	Messages(namespace string) MessageNamespaceLister
	MessageListerExpansion
}

// messageLister implements the MessageLister interface.
type messageLister struct {
	indexer cache.Indexer
}

// NewMessageLister returns a new MessageLister.
func NewMessageLister(indexer cache.Indexer) MessageLister {
	return &messageLister{indexer: indexer}
}

// List lists all Messages in the indexer.
func (s *messageLister) List(selector labels.Selector) (ret []*v1alpha1.Message, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.Message))
	})
	return ret, err
}

// Messages returns an object that can list and get Messages.
func (s *messageLister) Messages(namespace string) MessageNamespaceLister {
	return messageNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// MessageNamespaceLister helps list and get Messages.
type MessageNamespaceLister interface {
	// List lists all Messages in the indexer for a given namespace.
	List(selector labels.Selector) (ret []*v1alpha1.Message, err error)
	// Get retrieves the Message from the indexer for a given namespace and name.
	Get(name string) (*v1alpha1.Message, error)
	MessageNamespaceListerExpansion
}

// messageNamespaceLister implements the MessageNamespaceLister
// interface.
type messageNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all Messages in the indexer for a given namespace.
func (s messageNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.Message, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.Message))
	})
	return ret, err
}

// Get retrieves the Message from the indexer for a given namespace and name.
func (s messageNamespaceLister) Get(name string) (*v1alpha1.Message, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("message"), name)
	}
	return obj.(*v1alpha1.Message), nil
}
