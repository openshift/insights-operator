module github.com/openshift/insights-operator

go 1.14

require (
	github.com/getsentry/raven-go v0.2.1-0.20190513200303-c977f96e1095 // indirect
	github.com/go-logr/logr v0.3.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.1.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.11.3 // indirect
	github.com/openshift/api v0.0.0-20201214114959-164a2fb63b5f
	github.com/openshift/client-go v0.0.0-20201214125552-e615e336eb49
	github.com/openshift/library-go v0.0.0-20201214135256-d265f469e75b
	github.com/spf13/cobra v1.1.1
	github.com/spf13/pflag v1.0.5
	golang.org/x/net v0.0.0-20201209123823-ac852fbbde11
	golang.org/x/time v0.0.0-20200630173020-3af7569d3a1e
	gopkg.in/yaml.v2 v2.4.0 // indirect
	k8s.io/api v0.20.0
	k8s.io/apiextensions-apiserver v0.20.0
	k8s.io/apimachinery v0.20.0
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/component-base v0.20.0
	k8s.io/klog/v2 v2.4.0
)

replace (
	github.com/openshift/api => github.com/openshift/api v0.0.0-20201214114959-164a2fb63b5f
	github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20201214125552-e615e336eb49
	github.com/openshift/library-go => github.com/openshift/library-go v0.0.0-20201214135256-d265f469e75b
	k8s.io/api => k8s.io/api v0.20.0
	k8s.io/apimachinery => k8s.io/apimachinery v0.20.0
	k8s.io/apiserver => k8s.io/apiserver v0.20.0
	k8s.io/client-go => k8s.io/client-go v0.20.0
)
