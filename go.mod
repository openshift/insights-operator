module github.com/openshift/insights-operator

go 1.13

require (
	github.com/coreos/pkg v0.0.0-20180928190104-399ea9e2e55f // indirect
	github.com/getsentry/raven-go v0.2.1-0.20190513200303-c977f96e1095 // indirect
	github.com/gorilla/websocket v1.4.1 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.1.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.11.3 // indirect
	github.com/openshift/api v0.0.0-20200521101457-60c476765272
	github.com/openshift/client-go v0.0.0-20200521150516-05eb9880269c
	github.com/openshift/library-go v0.0.0-20201028085638-6dc5374cde18
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	github.com/tmc/grpc-websocket-proxy v0.0.0-20190109142713-0ad062ec5ee5 // indirect
	golang.org/x/net v0.0.0-20200421231249-e086a090c8fd
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4
	google.golang.org/appengine v1.6.1 // indirect
	k8s.io/api v0.18.6
	k8s.io/apimachinery v0.18.6
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/component-base v0.18.6
	k8s.io/klog v1.0.0
)

replace (
	github.com/openshift/api => github.com/openshift/api v0.0.0-20200521101457-60c476765272
	github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20200521150516-05eb9880269c
	github.com/openshift/library-go => github.com/openshift/library-go v0.0.0-20201028085638-6dc5374cde18
	k8s.io/api => k8s.io/api v0.18.6
	k8s.io/apimachinery => k8s.io/apimachinery v0.18.6
	k8s.io/apiserver => k8s.io/apiserver v0.18.6
	k8s.io/client-go => k8s.io/client-go v0.18.6
)
