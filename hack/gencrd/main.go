package main

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/appscode/go/log"
	gort "github.com/appscode/go/runtime"
	crdutils "github.com/appscode/kutil/apiextensions/v1beta1"
	"github.com/appscode/kutil/openapi"
	"github.com/go-openapi/spec"
	"github.com/golang/glog"
	"github.com/kubedb/user-manager/apis/authorization/install"
	"github.com/kubedb/user-manager/apis/authorization/v1alpha1"
	crd_api "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/kube-openapi/pkg/common"
)

func generateCRDDefinitions() {
	filename := gort.GOPath() + "/src/github.com/kubedb/user-manager/apis/authorization/v1alpha1/crds.yaml"
	os.Remove(filename)

	err := os.MkdirAll(filepath.Join(gort.GOPath(), "/src/github.com/kubedb/user-manager/api/crds"), 0755)
	if err != nil {
		log.Fatal(err)
	}

	crds := []*crd_api.CustomResourceDefinition{
		v1alpha1.PostgresRole{}.CustomResourceDefinition(),
		v1alpha1.PostgresRoleBinding{}.CustomResourceDefinition(),
		v1alpha1.MongoDBRole{}.CustomResourceDefinition(),
		v1alpha1.MongoDBRoleBinding{}.CustomResourceDefinition(),
		v1alpha1.MySQLRole{}.CustomResourceDefinition(),
		v1alpha1.MySQLRoleBinding{}.CustomResourceDefinition(),
	}
	for _, crd := range crds {
		filename := filepath.Join(gort.GOPath(), "/src/github.com/kubedb/user-manager/api/crds", crd.Spec.Names.Singular+".yaml")
		f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			log.Fatal(err)
		}
		crdutils.MarshallCrd(f, crd, "yaml")
		f.Close()
	}
}

func generateSwaggerJson() {
	var (
		Scheme = runtime.NewScheme()
		Codecs = serializer.NewCodecFactory(Scheme)
	)

	install.Install(Scheme)

	apispec, err := openapi.RenderOpenAPISpec(openapi.Config{
		Scheme: Scheme,
		Codecs: Codecs,
		Info: spec.InfoProps{
			Title:   "KubeDB User Manager",
			Version: "v1alpha1",
			Contact: &spec.ContactInfo{
				Name:  "AppsCode Inc.",
				URL:   "https://appscode.com",
				Email: "hello@appscode.com",
			},
			License: &spec.License{
				Name: "Apache 2.0",
				URL:  "https://www.apache.org/licenses/LICENSE-2.0.html",
			},
		},
		OpenAPIDefinitions: []common.GetOpenAPIDefinitions{
			v1alpha1.GetOpenAPIDefinitions,
		},
		Resources: []openapi.TypeInfo{
			{v1alpha1.SchemeGroupVersion, v1alpha1.ResourcePostgresRoles, v1alpha1.ResourceKindPostgresRole, true},
			{v1alpha1.SchemeGroupVersion, v1alpha1.ResourcePostgresRoleBindings, v1alpha1.ResourceKindPostgresRoleBinding, true},
			{v1alpha1.SchemeGroupVersion, v1alpha1.ResourceMongoDBRoles, v1alpha1.ResourceKindMongoDBRole, true},
			{v1alpha1.SchemeGroupVersion, v1alpha1.ResourceMongoDBRoleBindings, v1alpha1.ResourceKindMongoDBRoleBinding, true},
			{v1alpha1.SchemeGroupVersion, v1alpha1.ResourceMySQLRoles, v1alpha1.ResourceKindMySQLRole, true},
			{v1alpha1.SchemeGroupVersion, v1alpha1.ResourceMySQLRoleBindings, v1alpha1.ResourceKindMySQLRoleBinding, true},
		},
	})
	if err != nil {
		glog.Fatal(err)
	}

	filename := gort.GOPath() + "/src/github.com/kubedb/user-manager/api/openapi-spec/swagger.json"
	err = os.MkdirAll(filepath.Dir(filename), 0755)
	if err != nil {
		glog.Fatal(err)
	}
	err = ioutil.WriteFile(filename, []byte(apispec), 0644)
	if err != nil {
		glog.Fatal(err)
	}
}

func main() {
	generateCRDDefinitions()
	generateSwaggerJson()
}
