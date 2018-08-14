package framework

import (
	"fmt"
	"time"

	"github.com/appscode/go/crypto/rand"
	_ "github.com/lib/pq"
	. "github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	PostgresUser             = "postgres"
	PostgresPassword         = "root"
	PostgresCredentialSecret = "pg-credential-secret"
)

var (
	postgresqlServiceName    = rand.WithUniqSuffix("test-svc-postgresql")
	postgresqlDeploymentName = rand.WithUniqSuffix("test-postgresql-deploy")
)

// DeployPostgres will do:
//	- create service
//	- create deployment
//  - create credential secret
func (f *Framework) DeployPostgres() (string, error) {
	label := map[string]string{
		"app": rand.WithUniqSuffix("test-postgresql"),
	}

	srv := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: f.namespace,
			Name:      postgresqlServiceName,
		},
		Spec: corev1.ServiceSpec{
			Selector: label,
			Ports: []corev1.ServicePort{
				{
					Name:       "tcp",
					Protocol:   corev1.ProtocolTCP,
					Port:       5432,
					TargetPort: intstr.FromInt(5432),
				},
			},
		},
	}

	url := fmt.Sprintf("%s.%s.svc:5432", postgresqlServiceName, f.namespace)

	postgresqlCont := corev1.Container{
		Name:            "postgres",
		Image:           "postgres:9.6.2",
		ImagePullPolicy: "IfNotPresent",
		Env: []corev1.EnvVar{
			{
				Name:  "POSTGRES_USER",
				Value: PostgresUser,
			},
			{
				Name:  "POSTGRES_PASSWORD",
				Value: PostgresPassword,
			},
			{
				Name:  "POSTGRES_DB",
				Value: "database",
			},
			{
				Name:  "PGDATA",
				Value: "/var/lib/postgresql/data/pgdata",
			},
			{
				Name: "POD_IP",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "status.podIP",
					},
				},
			},
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          "postgresql",
				Protocol:      corev1.ProtocolTCP,
				ContainerPort: 5432,
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				MountPath: "/var/lib/postgresql/data/pgdata",
				Name:      "data",
				SubPath:   "postgresgl-db",
			},
		},
		ReadinessProbe: &corev1.Probe{
			Handler: corev1.Handler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"sh",
						"-c",
						"exec pg_isready --host $POD_IP",
					},
				},
			},
			InitialDelaySeconds: 5,
			TimeoutSeconds:      3,
			PeriodSeconds:       5,
			FailureThreshold:    10,
		},
	}

	postgresqlDeploy := apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: f.namespace,
			Name:      postgresqlDeploymentName,
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
						postgresqlCont,
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

	_, err = f.CreateDeployment(postgresqlDeploy)
	if err != nil {
		return "", err
	}

	Eventually(func() bool {
		if obj, err := f.KubeClient.AppsV1beta1().Deployments(f.namespace).Get(postgresqlDeploy.GetName(), metav1.GetOptions{}); err == nil {
			return *obj.Spec.Replicas == obj.Status.ReadyReplicas
		}
		return false
	}, timeOut, pollingInterval).Should(BeTrue())

	time.Sleep(10 * time.Second)

	sr := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PostgresCredentialSecret,
			Namespace: f.namespace,
		},
		Data: map[string][]byte{
			"username": []byte(PostgresUser),
			"password": []byte(PostgresPassword),
		},
	}

	_, err = f.KubeClient.CoreV1().Secrets(f.namespace).Create(sr)
	if err != nil {
		return "", err
	}

	return url, nil
}

func (f *Framework) DeletePostgres() error {
	err := f.DeleteService(postgresqlServiceName, f.namespace)
	if err != nil {
		return err
	}

	err = f.DeleteSecret(PostgresCredentialSecret, f.namespace)
	if err != nil {
		return err
	}

	err = f.DeleteDeployment(postgresqlDeploymentName, f.namespace)
	return err
}
