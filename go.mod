module github.com/openshift/insights-operator

go 1.12

require (
	github.com/RedHatInsights/insights-results-smart-proxy v0.0.0-20200821102133-8859353f04ca
	github.com/getsentry/raven-go v0.2.1-0.20190513200303-c977f96e1095 // indirect
	github.com/gogo/protobuf v1.3.1 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.1.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.11.3 // indirect
	github.com/openshift/api v0.0.0-20200117162508-e7ccdda6ba67
	github.com/openshift/client-go v0.0.0-20200116152001-92a2713fa240
	github.com/openshift/library-go v0.0.0-20200120153246-906409ae5e38
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	golang.org/x/net v0.0.0-20200625001655-4c5254603344
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0
	k8s.io/api v0.17.1
	k8s.io/apimachinery v0.17.1
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/component-base v0.17.1
	k8s.io/klog v1.0.0
)

replace (
	github.com/openshift/api => github.com/openshift/api v0.0.0-20200117162508-e7ccdda6ba67
	github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20200116152001-92a2713fa240
	github.com/openshift/library-go => github.com/openshift/library-go v0.0.0-20200120153246-906409ae5e38
	k8s.io/api => k8s.io/api v0.17.1
	k8s.io/apimachinery => k8s.io/apimachinery v0.17.1
	k8s.io/apiserver => k8s.io/apiserver v0.17.1
	k8s.io/client-go => k8s.io/client-go v0.17.1
)
