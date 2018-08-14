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

func CreateOrPatchMysqlRole(c cs.AuthorizationV1alpha1Interface, meta metav1.ObjectMeta, transform func(alert *api.MysqlRole) *api.MysqlRole) (*api.MysqlRole, kutil.VerbType, error) {
	cur, err := c.MysqlRoles(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
	if kerr.IsNotFound(err) {
		glog.V(3).Infof("Creating MysqlRole %s/%s.", meta.Namespace, meta.Name)
		out, err := c.MysqlRoles(meta.Namespace).Create(transform(&api.MysqlRole{
			TypeMeta: metav1.TypeMeta{
				Kind:       api.ResourceKindMysqlRole,
				APIVersion: api.SchemeGroupVersion.String(),
			},
			ObjectMeta: meta,
		}))
		return out, kutil.VerbCreated, err
	} else if err != nil {
		return nil, kutil.VerbUnchanged, err
	}
	return PatchMysqlRole(c, cur, transform)
}

func PatchMysqlRole(c cs.AuthorizationV1alpha1Interface, cur *api.MysqlRole, transform func(*api.MysqlRole) *api.MysqlRole) (*api.MysqlRole, kutil.VerbType, error) {
	return PatchMysqlRoleObject(c, cur, transform(cur.DeepCopy()))
}

func PatchMysqlRoleObject(c cs.AuthorizationV1alpha1Interface, cur, mod *api.MysqlRole) (*api.MysqlRole, kutil.VerbType, error) {
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
	glog.V(3).Infof("Patching MysqlRole %s/%s with %s.", cur.Namespace, cur.Name, string(patch))
	out, err := c.MysqlRoles(cur.Namespace).Patch(cur.Name, types.MergePatchType, patch)
	return out, kutil.VerbPatched, err
}

func TryUpdateMysqlRole(c cs.AuthorizationV1alpha1Interface, meta metav1.ObjectMeta, transform func(*api.MysqlRole) *api.MysqlRole) (result *api.MysqlRole, err error) {
	attempt := 0
	err = wait.PollImmediate(kutil.RetryInterval, kutil.RetryTimeout, func() (bool, error) {
		attempt++
		cur, e2 := c.MysqlRoles(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
		if kerr.IsNotFound(e2) {
			return false, e2
		} else if e2 == nil {
			result, e2 = c.MysqlRoles(cur.Namespace).Update(transform(cur.DeepCopy()))
			return e2 == nil, nil
		}
		glog.Errorf("Attempt %d failed to update MysqlRole %s/%s due to %v.", attempt, cur.Namespace, cur.Name, e2)
		return false, nil
	})

	if err != nil {
		err = errors.Errorf("failed to update MysqlRole %s/%s after %d attempts due to %v", meta.Namespace, meta.Name, attempt, err)
	}
	return
}

func UpdateMysqlRoleStatus(c cs.AuthorizationV1alpha1Interface, cur *api.MysqlRole, transform func(*api.MysqlRoleStatus) *api.MysqlRoleStatus, useSubresource ...bool) (*api.MysqlRole, error) {
	if len(useSubresource) > 1 {
		return nil, errors.Errorf("invalid value passed for useSubresource: %v", useSubresource)
	}

	mod := &api.MysqlRole{
		TypeMeta:   cur.TypeMeta,
		ObjectMeta: cur.ObjectMeta,
		Spec:       cur.Spec,
		Status:     *transform(cur.Status.DeepCopy()),
	}

	if len(useSubresource) == 1 && useSubresource[0] {
		return c.MysqlRoles(cur.Namespace).UpdateStatus(mod)
	}

	out, _, err := PatchMysqlRoleObject(c, cur, mod)
	return out, err
}
