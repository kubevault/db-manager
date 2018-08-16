package framework

import (
	"fmt"
	"time"

	"github.com/appscode/go/crypto/rand"
	. "github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	MongodbUser             = "root"
	MongodbPassword         = "root"
	MongodbCredentialSecret = "mongodb-credential-secret"
)

var (
	MongodbServiceName    = rand.WithUniqSuffix("test-svc-mongodb")
	MongodbDeploymentName = rand.WithUniqSuffix("test-mongodb-deploy")
)

// DeployMongodb will do:
//	- create service
//	- create deployment
//  - create credential secret
func (f *Framework) DeployMongodb() (string, error) {
	label := map[string]string{
		"app": rand.WithUniqSuffix("test-mongodb"),
	}

	srv := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: f.namespace,
			Name:      MongodbServiceName,
		},
		Spec: corev1.ServiceSpec{
			Selector: label,
			Ports: []corev1.ServicePort{
				{
					Name:       "tcp",
					Protocol:   corev1.ProtocolTCP,
					Port:       27017,
					TargetPort: intstr.FromInt(27017),
				},
			},
		},
	}

	url := fmt.Sprintf("%s.%s.svc:27017", MongodbServiceName, f.namespace)

	mongodbCont := corev1.Container{
		Name:            "mongo",
		Image:           "mongo",
		ImagePullPolicy: "IfNotPresent",
		Env: []corev1.EnvVar{
			{
				Name:  "MONGO_INITDB_ROOT_USERNAME",
				Value: MongodbUser,
			},
			{
				Name:  "MONGO_INITDB_ROOT_PASSWORD",
				Value: MongodbPassword,
			},
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          "mongodb",
				Protocol:      corev1.ProtocolTCP,
				ContainerPort: 27017,
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				MountPath: "/data/db",
				Name:      "data",
			},
		},
	}

	mongodbDeploy := apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: f.namespace,
			Name:      MongodbDeploymentName,
		},
		Spec: apps.DeploymentSpec{
			Replicas: func(i int32) *int32 { return &i }(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: label,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: label,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						mongodbCont,
					},
					Volumes: []corev1.Volume{
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	err := f.CreateService(srv)
	if err != nil {
		return "", err
	}

	_, err = f.CreateDeployment(mongodbDeploy)
	if err != nil {
		return "", err
	}

	Eventually(func() bool {
		if obj, err := f.KubeClient.AppsV1beta1().Deployments(f.namespace).Get(mongodbDeploy.GetName(), metav1.GetOptions{}); err == nil {
			return *obj.Spec.Replicas == obj.Status.ReadyReplicas
		}
		return false
	}, timeOut, pollingInterval).Should(BeTrue())

	time.Sleep(10 * time.Second)

	sr := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MongodbCredentialSecret,
			Namespace: f.namespace,
		},
		Data: map[string][]byte{
			"username": []byte(MongodbUser),
			"password": []byte(MongodbPassword),
		},
	}

	_, err = f.KubeClient.CoreV1().Secrets(f.namespace).Create(sr)
	if err != nil {
		return "", err
	}

	return url, nil
}

func (f *Framework) DeleteMongodb() error {
	err := f.DeleteService(MongodbServiceName, f.namespace)
	if err != nil {
		return err
	}

	err = f.DeleteSecret(MongodbCredentialSecret, f.namespace)
	if err != nil {
		return err
	}

	err = f.DeleteDeployment(MongodbDeploymentName, f.namespace)
	return err
}
