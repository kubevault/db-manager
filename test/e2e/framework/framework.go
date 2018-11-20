package framework

import (
	"time"

	"fmt"
	"github.com/appscode/go/crypto/rand"
	aggregator "github.com/appscode/go/util/errors"
	cs "github.com/kubedb/apimachinery/client/clientset/versioned"
	crd_cs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ka "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset"
)

const (
	timeOut         = 10 * time.Minute
	pollingInterval = 10 * time.Second
)

type Framework struct {
	KubeClient kubernetes.Interface
	DBClient   cs.Interface
	CRDClient  crd_cs.ApiextensionsV1beta1Interface
	KAClient   ka.Interface

	namespace      string
	WebhookEnabled bool

	ClientConfig *rest.Config

	PostgresUrl string
	MysqlUrl    string
	MongodbUrl  string
	VaultUrl    string
}

func New(
	kubeClient kubernetes.Interface,
	dbClient cs.Interface,
	crdClient crd_cs.ApiextensionsV1beta1Interface,
	kaClient ka.Interface,
	webhookEnabled bool,
	clientConfig *rest.Config) *Framework {

	return &Framework{
		KubeClient: kubeClient,
		DBClient:   dbClient,
		CRDClient:  crdClient,
		KAClient:   kaClient,

		namespace:      rand.WithUniqSuffix("user-manager-e2e"),
		WebhookEnabled: webhookEnabled,

		ClientConfig: clientConfig,
	}
}

func (f *Framework) InitialSetup() error {
	var err error
	f.PostgresUrl, err = f.DeployPostgres()
	if err != nil {
		return err
	}

	f.MysqlUrl, err = f.DeployMysql()
	if err != nil {
		return err
	}

	f.MongodbUrl, err = f.DeployMongodb()
	if err != nil {
		return err
	}

	f.VaultUrl, err = f.DeployVault()
	if err != nil {
		return err
	}

	fmt.Println(f.VaultUrl)

	return nil
}

func (f *Framework) Cleanup() error {
	errs := []error{}

	err := f.DeletePostgres()
	if err != nil {
		errs = append(errs, err)
	}

	err = f.DeleteMysql()
	if err != nil {
		errs = append(errs, err)
	}

	err = f.DeleteMongodb()
	if err != nil {
		errs = append(errs, err)
	}

	err = f.DeleteVault()
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) != 0 {
		return aggregator.NewAggregate(errs)
	}
	return nil
}

func (f *Framework) Invoke() *Invocation {
	return &Invocation{
		Framework: f,
		app:       rand.WithUniqSuffix("test-user-manager"),
	}
}

type Invocation struct {
	*Framework
	app string
}

func (f *Invocation) App() string {
	return f.app
}
