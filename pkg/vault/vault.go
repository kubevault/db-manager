package vault

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/url"

	vaultapi "github.com/hashicorp/vault/api"
	_ "github.com/kubedb/apimachinery/apis/authorization/v1alpha1"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	appcat "kmodules.xyz/custom-resources/apis/appcatalog/v1alpha1"
)

func NewClient(kclient kubernetes.Interface, binding *appcat.AppBinding) (*vaultapi.Client, error) {
	cfg, err := newVaultConfig(kclient, namespace, binding)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create vault client config")
	}

	cl, err := vaultapi.NewClient(cfg)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	sr, err := kclient.CoreV1().Secrets(namespace).Get(binding.Spec.Secret.Name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get vault token secret %s/%s", namespace, binding.TokenSecret)
	}

	if sr.Data == nil {
		return nil, errors.Errorf("vault token is not found in secret %s/%s")
	}
	if _, ok := sr.Data["token"]; !ok {
		return nil, errors.Errorf("vault token is not found in secret %s/%s")
	}
	cl.SetToken(string(sr.Data["token"]))

	return cl, nil
}

func newVaultConfig(kclient kubernetes.Interface, binding *appcat.AppBinding) (*vaultapi.Config, error) {
	cfg := vaultapi.DefaultConfig()
	cfg.Address = binding.Address

	clientTLSConfig := cfg.HttpClient.Transport.(*http.Transport).TLSClientConfig

	if binding.Spec.Secret.Name != "" {
		sr, err := kclient.CoreV1().Secrets(namespace).Get(binding.Spec.Secret.Name, metav1.GetOptions{})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get vault client tls secret %s/%s", namespace, binding.Spec.Secret.Name)
		}

		clientTLSConfig.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			cert, err := tls.X509KeyPair(sr.Data["client.crt"], sr.Data["client.key"])
			if err != nil {
				return nil, errors.WithStack(err)
			}
			return &cert, nil
		}
	}

	if binding.Spec.ClientConfig.InsecureSkipTLSVerify {
		clientTLSConfig.InsecureSkipVerify = true
	} else {
		pool := x509.NewCertPool()
		ok := pool.AppendCertsFromPEM(binding.Spec.ClientConfig.CABundle)
		if !ok {
			return nil, errors.New("error loading CA bundle")
		}
		clientTLSConfig.RootCAs = pool
	}

	var err error
	clientTLSConfig.ServerName, err = getHostName(binding.Address)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get hostname from url %s", binding.Address)
	}

	return cfg, nil
}

func getHostName(addr string) (string, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return u.Hostname(), nil
}
