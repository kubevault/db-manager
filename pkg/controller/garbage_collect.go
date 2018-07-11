package controller

import (
	"time"

	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *MessengerController) garbageCollect(stopCh <-chan struct{}, gcTime time.Duration) {
	for {
		var stop bool

		messages, err := c.authzClient.AuthorizationV1alpha1().Messages(metav1.NamespaceAll).List(metav1.ListOptions{})
		if err == nil {
			doneC := make(chan bool, 1)
			// delete messages in a go routine
			go func() {
				for _, msg := range messages.Items {
					age := msg.CreationTimestamp.Add(gcTime)
					if age.Before(time.Now()) {
						c.authzClient.AuthorizationV1alpha1().Messages(msg.Namespace).Delete(msg.Name, deleteInBackground())
					}
				}
				doneC <- true
			}()

			for done := false; !done && !stop; {
				select {
				case <-doneC:
					done = true
				case <-stopCh:
					stop = true
				}
			}

			if stop {
				break
			}
		}

		time.Sleep(time.Minute)
	}

	// Stop collecting messages
	glog.Info("Shutting down garbage collector of Message CRD")
}

func deleteInBackground() *metav1.DeleteOptions {
	policy := metav1.DeletePropagationBackground
	return &metav1.DeleteOptions{PropagationPolicy: &policy}
}
