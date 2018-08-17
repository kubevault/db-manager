package util

import (
	"encoding/json"

	"github.com/appscode/kutil"
	"github.com/evanphx/json-patch"
	"github.com/golang/glog"
	api "github.com/kubedb/user-manager/apis/authorization/v1alpha1"
	cs "github.com/kubedb/user-manager/client/clientset/versioned/typed/authorization/v1alpha1"
	"github.com/pkg/errors"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
)

func CreateOrPatchMongodbRoleBinding(c cs.AuthorizationV1alpha1Interface, meta metav1.ObjectMeta, transform func(alert *api.MongodbRoleBinding) *api.MongodbRoleBinding) (*api.MongodbRoleBinding, kutil.VerbType, error) {
	cur, err := c.MongodbRoleBindings(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
	if kerr.IsNotFound(err) {
		glog.V(3).Infof("Creating MongodbRoleBinding %s/%s.", meta.Namespace, meta.Name)
		out, err := c.MongodbRoleBindings(meta.Namespace).Create(transform(&api.MongodbRoleBinding{
			TypeMeta: metav1.TypeMeta{
				Kind:       api.ResourceKindMongodbRoleBinding,
				APIVersion: api.SchemeGroupVersion.String(),
			},
			ObjectMeta: meta,
		}))
		return out, kutil.VerbCreated, err
	} else if err != nil {
		return nil, kutil.VerbUnchanged, err
	}
	return PatchMongodbRoleBinding(c, cur, transform)
}

func PatchMongodbRoleBinding(c cs.AuthorizationV1alpha1Interface, cur *api.MongodbRoleBinding, transform func(*api.MongodbRoleBinding) *api.MongodbRoleBinding) (*api.MongodbRoleBinding, kutil.VerbType, error) {
	return PatchMongodbRoleBindingObject(c, cur, transform(cur.DeepCopy()))
}

func PatchMongodbRoleBindingObject(c cs.AuthorizationV1alpha1Interface, cur, mod *api.MongodbRoleBinding) (*api.MongodbRoleBinding, kutil.VerbType, error) {
	curJson, err := json.Marshal(cur)
	if err != nil {
		return nil, kutil.VerbUnchanged, err
	}

	modJson, err := json.Marshal(mod)
	if err != nil {
		return nil, kutil.VerbUnchanged, err
	}

	patch, err := jsonpatch.CreateMergePatch(curJson, modJson)
	if err != nil {
		return nil, kutil.VerbUnchanged, err
	}
	if len(patch) == 0 || string(patch) == "{}" {
		return cur, kutil.VerbUnchanged, nil
	}
	glog.V(3).Infof("Patching MongodbRoleBinding %s/%s with %s.", cur.Namespace, cur.Name, string(patch))
	out, err := c.MongodbRoleBindings(cur.Namespace).Patch(cur.Name, types.MergePatchType, patch)
	return out, kutil.VerbPatched, err
}

func TryUpdateMongodbRoleBinding(c cs.AuthorizationV1alpha1Interface, meta metav1.ObjectMeta, transform func(*api.MongodbRoleBinding) *api.MongodbRoleBinding) (result *api.MongodbRoleBinding, err error) {
	attempt := 0
	err = wait.PollImmediate(kutil.RetryInterval, kutil.RetryTimeout, func() (bool, error) {
		attempt++
		cur, e2 := c.MongodbRoleBindings(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
		if kerr.IsNotFound(e2) {
			return false, e2
		} else if e2 == nil {
			result, e2 = c.MongodbRoleBindings(cur.Namespace).Update(transform(cur.DeepCopy()))
			return e2 == nil, nil
		}
		glog.Errorf("Attempt %d failed to update MongodbRoleBinding %s/%s due to %v.", attempt, cur.Namespace, cur.Name, e2)
		return false, nil
	})

	if err != nil {
		err = errors.Errorf("failed to update MongodbRoleBinding %s/%s after %d attempts due to %v", meta.Namespace, meta.Name, attempt, err)
	}
	return
}

func UpdateMongodbRoleBindingStatus(c cs.AuthorizationV1alpha1Interface, cur *api.MongodbRoleBinding, transform func(*api.MongodbRoleBindingStatus) *api.MongodbRoleBindingStatus, useSubresource ...bool) (*api.MongodbRoleBinding, error) {
	if len(useSubresource) > 1 {
		return nil, errors.Errorf("invalid value passed for useSubresource: %v", useSubresource)
	}

	mod := &api.MongodbRoleBinding{
		TypeMeta:   cur.TypeMeta,
		ObjectMeta: cur.ObjectMeta,
		Spec:       cur.Spec,
		Status:     *transform(cur.Status.DeepCopy()),
	}

	if len(useSubresource) == 1 && useSubresource[0] {
		return c.MongodbRoleBindings(cur.Namespace).UpdateStatus(mod)
	}

	out, _, err := PatchMongodbRoleBindingObject(c, cur, mod)
	return out, err
}
