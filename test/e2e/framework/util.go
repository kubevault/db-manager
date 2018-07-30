package framework

import (
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
	"time"
)

func deleteInBackground() *metav1.DeleteOptions {
	policy := metav1.DeletePropagationBackground
	return &metav1.DeleteOptions{PropagationPolicy: &policy}
}

func newObjectMeta(name, namespace string, labels map[string]string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
		Labels:    labels,
	}
}

func newObjectMetaWithGenerateName(genName, namespace string, labels map[string]string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		GenerateName: genName,
		Namespace:    namespace,
		Labels:       labels,
	}
}

func GetDateString(t time.Time) string {
	var res string
	parts := strings.Split(t.String(), " ")
	res += parts[0]
	res += "T" + parts[1]
	res += "%2B" + parts[2][1:3] + ":" + parts[2][3:]
	fmt.Printf(">>>>>>>>>>>>> Date string=%s\n", res)
	return res
}
