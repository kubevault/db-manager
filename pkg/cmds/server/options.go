package server

import (
	"flag"
	"time"

	"github.com/kubedb/apimachinery/apis"
	cs "github.com/kubedb/apimachinery/client/clientset/versioned"
	"github.com/kubedb/user-manager/pkg/controller"
	"github.com/spf13/pflag"
	crd_cs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	"k8s.io/client-go/kubernetes"
	appcat_cs "kmodules.xyz/custom-resources/client/clientset/versioned/typed/appcatalog/v1alpha1"
)

type ExtraOptions struct {
	MaxNumRequeues int
	NumThreads     int
	QPS            float64
	Burst          int
	ResyncPeriod   time.Duration
	LeaseRenewTime time.Duration
}

func NewExtraOptions() *ExtraOptions {
	return &ExtraOptions{
		MaxNumRequeues: 5,
		NumThreads:     2,
		QPS:            100,
		Burst:          100,
		ResyncPeriod:   10 * time.Minute,
		LeaseRenewTime: 30 * time.Minute,
	}
}

func (s *ExtraOptions) AddGoFlags(fs *flag.FlagSet) {
	fs.Float64Var(&s.QPS, "qps", s.QPS, "The maximum QPS to the master from this client")
	fs.IntVar(&s.Burst, "burst", s.Burst, "The maximum burst for throttle")
	fs.DurationVar(&s.ResyncPeriod, "resync-period", s.ResyncPeriod, "If non-zero, will re-list this often. Otherwise, re-list will be delayed aslong as possible (until the upstream source closes the watch or times out.")
	fs.DurationVar(&s.LeaseRenewTime, "lease-renew-time", s.LeaseRenewTime, "lease renewer will renew the lease after every lease renew time")
	fs.BoolVar(&apis.EnableStatusSubresource, "enable-status-subresource", apis.EnableStatusSubresource, "If true, uses sub resource for Voyager crds.")
}

func (s *ExtraOptions) AddFlags(fs *pflag.FlagSet) {
	pfs := flag.NewFlagSet("messenger", flag.ExitOnError)
	s.AddGoFlags(pfs)
	fs.AddGoFlagSet(pfs)
}

func (s *ExtraOptions) ApplyTo(cfg *controller.Config) error {
	var err error

	cfg.MaxNumRequeues = s.MaxNumRequeues
	cfg.NumThreads = s.NumThreads
	cfg.ResyncPeriod = s.ResyncPeriod
	cfg.LeaseRenewTime = s.LeaseRenewTime

	cfg.ClientConfig.QPS = float32(s.QPS)
	cfg.ClientConfig.Burst = s.Burst

	if cfg.KubeClient, err = kubernetes.NewForConfig(cfg.ClientConfig); err != nil {
		return err
	}
	if cfg.DbClient, err = cs.NewForConfig(cfg.ClientConfig); err != nil {
		return err
	}
	if cfg.CRDClient, err = crd_cs.NewForConfig(cfg.ClientConfig); err != nil {
		return err
	}
	if cfg.AppCatalogClient, err = appcat_cs.NewForConfig(cfg.ClientConfig); err != nil {
		return err
	}
	return nil
}
