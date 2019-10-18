module github.com/openshift/insights-operator

go 1.12

require (
	github.com/certifi/gocertifi v0.0.0-20180905225744-ee1a9a0726d2 // indirect
	github.com/coreos/bbolt v1.3.3 // indirect
	github.com/coreos/pkg v0.0.0-20180928190104-399ea9e2e55f // indirect
	github.com/eparis/urlhash v0.0.0-20191022152723-5838cc59d83c
	github.com/getsentry/raven-go v0.2.1-0.20190513200303-c977f96e1095 // indirect
	github.com/google/btree v1.0.0 // indirect
	github.com/gorilla/websocket v1.4.1 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.1.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.11.3 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/onsi/ginkgo v1.10.2 // indirect
	github.com/onsi/gomega v1.7.0 // indirect
	github.com/openshift/api v3.9.1-0.20190718162300-9525304a0adb+incompatible
	github.com/openshift/client-go v0.0.0-20190627172412-c44a8b61b9f4
	github.com/openshift/library-go v0.0.0-20190717192022-83581c469d2e
	github.com/pkg/profile v1.3.0 // indirect
	github.com/prometheus/client_golang v0.9.2
	github.com/prometheus/common v0.2.0 // indirect
	github.com/soheilhy/cmux v0.1.4 // indirect
	github.com/spf13/cobra v0.0.2-0.20180319062004-c439c4fa0937
	github.com/spf13/pflag v1.0.3
	github.com/tmc/grpc-websocket-proxy v0.0.0-20190109142713-0ad062ec5ee5 // indirect
	github.com/xiang90/probing v0.0.0-20190116061207-43a291ad63a2 // indirect
	go.etcd.io/bbolt v1.3.3 // indirect
	golang.org/x/time v0.0.0-20181108054448-85acf8d2951c
	google.golang.org/appengine v1.6.1 // indirect
	k8s.io/api v0.0.0-20191016110408-35e52d86657a
	k8s.io/apimachinery v0.0.0-20191004115801-a2eda9f80ab8
	k8s.io/apiserver v0.0.0-20190313205120-8b27c41bdbb1 // indirect
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/component-base v0.0.0-20191016111319-039242c015a9
	k8s.io/klog v0.4.0
)

replace k8s.io/api => k8s.io/api v0.0.0-20191016110408-35e52d86657a

replace k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20191004115801-a2eda9f80ab8

replace k8s.io/apiserver => k8s.io/apiserver v0.0.0-20191016112112-5190913f932d

replace k8s.io/client-go => k8s.io/client-go v0.0.0-20191016111102-bec269661e48

replace k8s.io/component-base => k8s.io/component-base v0.0.0-20191016111319-039242c015a9

replace github.com/openshift/api => github.com/openshift/api v0.0.0-20191031084152-11eee842dafd

replace github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20191022152013-2823239d2298

replace github.com/openshift/library-go => github.com/openshift/library-go v0.0.0-20191030100914-d6f3b526f033
