package server

import (
	"fmt"
	"io"
	"net"

	"github.com/kubedb/user-manager/apis/authorization/v1alpha1"
	"github.com/kubedb/user-manager/pkg/controller"
	"github.com/kubedb/user-manager/pkg/server"
	"github.com/spf13/pflag"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	openapinamer "k8s.io/apiserver/pkg/endpoints/openapi"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"
)

const defaultEtcdPathPrefix = "/registry/authorization.kubedb.com"

type UserManagerOptions struct {
	RecommendedOptions *genericoptions.RecommendedOptions
	ExtraOptions       *ExtraOptions

	StdOut io.Writer
	StdErr io.Writer
}

func NewUserManagerOptions(out, errOut io.Writer) *UserManagerOptions {
	o := &UserManagerOptions{
		// TODO we will nil out the etcd storage options.  This requires a later level of k8s.io/apiserver
		RecommendedOptions: genericoptions.NewRecommendedOptions(defaultEtcdPathPrefix, server.Codecs.LegacyCodec(admissionv1beta1.SchemeGroupVersion)),
		ExtraOptions:       NewExtraOptions(),
		StdOut:             out,
		StdErr:             errOut,
	}
	o.RecommendedOptions.Etcd = nil
	o.RecommendedOptions.Admission = nil

	return o
}

func (o UserManagerOptions) AddFlags(fs *pflag.FlagSet) {
	o.RecommendedOptions.AddFlags(fs)
	o.ExtraOptions.AddFlags(fs)
}

func (o UserManagerOptions) Validate(args []string) error {
	return nil
}

func (o *UserManagerOptions) Complete() error {
	return nil
}

func (o UserManagerOptions) Config() (*server.UserManagerConfig, error) {
	// TODO have a "real" external address
	if err := o.RecommendedOptions.SecureServing.MaybeDefaultWithSelfSignedCerts("localhost", nil, []net.IP{net.ParseIP("127.0.0.1")}); err != nil {
		return nil, fmt.Errorf("error creating self-signed certificates: %v", err)
	}

	serverConfig := genericapiserver.NewRecommendedConfig(server.Codecs)
	if err := o.RecommendedOptions.ApplyTo(serverConfig, server.Scheme); err != nil {
		return nil, err
	}
	serverConfig.OpenAPIConfig = genericapiserver.DefaultOpenAPIConfig(v1alpha1.GetOpenAPIDefinitions, openapinamer.NewDefinitionNamer(server.Scheme))
	serverConfig.OpenAPIConfig.Info.Title = "messenger"
	serverConfig.OpenAPIConfig.Info.Version = v1alpha1.SchemeGroupVersion.Version
	serverConfig.OpenAPIConfig.IgnorePrefixes = []string{
		"/swaggerapi",
		"/apis/admission.authorization.kubedb.com/v1alpha1/messages",
	}

	extraConfig := controller.NewConfig(serverConfig.ClientConfig)
	if err := o.ExtraOptions.ApplyTo(extraConfig); err != nil {
		return nil, err
	}

	config := &server.UserManagerConfig{
		GenericConfig: serverConfig,
		ExtraConfig:   extraConfig,
	}
	return config, nil
}

func (o UserManagerOptions) Run(stopCh <-chan struct{}) error {
	config, err := o.Config()
	if err != nil {
		return err
	}

	s, err := config.Complete().New()
	if err != nil {
		return err
	}

	return s.Run(stopCh)
}
