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

func CreateOrPatchPostgresRole(c cs.AuthorizationV1alpha1Interface, meta metav1.ObjectMeta, transform func(alert *api.PostgresRole) *api.PostgresRole) (*api.PostgresRole, kutil.VerbType, error) {
	cur, err := c.PostgresRoles(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
	if kerr.IsNotFound(err) {
		glog.V(3).Infof("Creating PostgresRole %s/%s.", meta.Namespace, meta.Name)
		out, err := c.PostgresRoles(meta.Namespace).Create(transform(&api.PostgresRole{
			TypeMeta: metav1.TypeMeta{
				Kind:       api.ResourceKindPostgresRole,
				APIVersion: api.SchemeGroupVersion.String(),
			},
			ObjectMeta: meta,
		}))
		return out, kutil.VerbCreated, err
	} else if err != nil {
		return nil, kutil.VerbUnchanged, err
	}
	return PatchPostgresRole(c, cur, transform)
}

func PatchPostgresRole(c cs.AuthorizationV1alpha1Interface, cur *api.PostgresRole, transform func(*api.PostgresRole) *api.PostgresRole) (*api.PostgresRole, kutil.VerbType, error) {
	return PatchPostgresRoleObject(c, cur, transform(cur.DeepCopy()))
}

func PatchPostgresRoleObject(c cs.AuthorizationV1alpha1Interface, cur, mod *api.PostgresRole) (*api.PostgresRole, kutil.VerbType, error) {
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
	glog.V(3).Infof("Patching PostgresRole %s/%s with %s.", cur.Namespace, cur.Name, string(patch))
	out, err := c.PostgresRoles(cur.Namespace).Patch(cur.Name, types.MergePatchType, patch)
	return out, kutil.VerbPatched, err
}

func TryUpdatePostgresRole(c cs.AuthorizationV1alpha1Interface, meta metav1.ObjectMeta, transform func(*api.PostgresRole) *api.PostgresRole) (result *api.PostgresRole, err error) {
	attempt := 0
	err = wait.PollImmediate(kutil.RetryInterval, kutil.RetryTimeout, func() (bool, error) {
		attempt++
		cur, e2 := c.PostgresRoles(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
		if kerr.IsNotFound(e2) {
			return false, e2
		} else if e2 == nil {
			result, e2 = c.PostgresRoles(cur.Namespace).Update(transform(cur.DeepCopy()))
			return e2 == nil, nil
		}
		glog.Errorf("Attempt %d failed to update PostgresRole %s/%s due to %v.", attempt, cur.Namespace, cur.Name, e2)
		return false, nil
	})

	if err != nil {
		err = errors.Errorf("failed to update PostgresRole %s/%s after %d attempts due to %v", meta.Namespace, meta.Name, attempt, err)
	}
	return
}

func UpdatePostgresRoleStatus(c cs.AuthorizationV1alpha1Interface, cur *api.PostgresRole, transform func(*api.PostgresRoleStatus) *api.PostgresRoleStatus, useSubresource ...bool) (*api.PostgresRole, error) {
	if len(useSubresource) > 1 {
		return nil, errors.Errorf("invalid value passed for useSubresource: %v", useSubresource)
	}

	mod := &api.PostgresRole{
		TypeMeta:   cur.TypeMeta,
		ObjectMeta: cur.ObjectMeta,
		Spec:       cur.Spec,
		Status:     *transform(cur.Status.DeepCopy()),
	}

	if len(useSubresource) == 1 && useSubresource[0] {
		return c.PostgresRoles(cur.Namespace).UpdateStatus(mod)
	}

	out, _, err := PatchPostgresRoleObject(c, cur, mod)
	return out, err
}
