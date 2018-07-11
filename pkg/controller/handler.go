package controller

import (
	api "github.com/kubedb/user-manager/apis/authorization/v1alpha1"
)

func (c *MessengerController) enqueueUpdate(oldObj, newObj interface{}) bool {
	old := oldObj.(*api.Message)
	new := newObj.(*api.Message)

	if new.Status.SentTimestamp == nil ||
		old.Spec.Message != new.Spec.Message ||
		old.Spec.Chat != new.Spec.Chat ||
		old.Spec.Email != new.Spec.Email ||
		old.Spec.Sms != new.Spec.Sms ||
		old.Spec.Service != new.Spec.Service {

		return true
	}

	return false
}
