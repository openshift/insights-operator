module github.com/openshift/insights-operator

go 1.14

require (
	github.com/getsentry/raven-go v0.2.1-0.20190513200303-c977f96e1095 // indirect
	github.com/go-logr/logr v0.3.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/gorilla/websocket v1.4.1 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.1.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.11.3 // indirect
	github.com/openshift/api v0.0.0-20201204152819-09f84eef6831
	github.com/openshift/client-go v0.0.0-20201203191154-ae1d036a57aa
	github.com/openshift/library-go v0.0.0-20201204145946-e542ffd73c6f
	github.com/spf13/cobra v1.0.0
	github.com/spf13/pflag v1.0.5
	golang.org/x/net v0.0.0-20201202161906-c7110b5ffcbb
	golang.org/x/text v0.3.4 // indirect
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0
	gopkg.in/yaml.v2 v2.4.0 // indirect
	k8s.io/api v0.19.4
	k8s.io/apiextensions-apiserver v0.19.4
	k8s.io/apimachinery v0.19.4
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/component-base v0.19.4
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.4.0 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.0.2 // indirect
)

replace (
	github.com/openshift/api => github.com/openshift/api v0.0.0-20201204152819-09f84eef6831
	github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20201203191154-ae1d036a57aa
	github.com/openshift/library-go => github.com/openshift/library-go v0.0.0-20201204145946-e542ffd73c6f
	k8s.io/api => k8s.io/api v0.19.4
	k8s.io/apimachinery => k8s.io/apimachinery v0.19.4
	k8s.io/apiserver => k8s.io/apiserver v0.19.4
	k8s.io/client-go => k8s.io/client-go v0.19.4
)
