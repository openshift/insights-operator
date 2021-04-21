module github.com/openshift/insights-operator

go 1.16

require (
	github.com/golang/groupcache v0.0.0-20200121045136-8c9f03a8e57e
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.1.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.11.3 // indirect
	github.com/openshift/api v0.0.0-20210409143810-a99ffa1cac67
	github.com/openshift/client-go v0.0.0-20210409155308-a8e62c60e930
	github.com/openshift/library-go v0.0.0-20210414082648-6e767630a0dc
	github.com/spf13/cobra v1.1.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	golang.org/x/net v0.0.0-20210224082022-3d97a244fca7
	golang.org/x/time v0.0.0-20210220033141-f8bda1e9f3ba
	k8s.io/api v0.21.0
	k8s.io/apiextensions-apiserver v0.21.0
	k8s.io/apimachinery v0.21.0
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/component-base v0.21.0
	k8s.io/klog/v2 v2.8.0
	k8s.io/utils v0.0.0-20201110183641-67b214c5f920
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/openshift/api => github.com/openshift/api v0.0.0-20201214114959-164a2fb63b5f
	github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20210409155308-a8e62c60e930
	github.com/openshift/library-go => github.com/openshift/library-go v0.0.0-20210414082648-6e767630a0dc
	k8s.io/api => k8s.io/api v0.21.0
	k8s.io/apimachinery => k8s.io/apimachinery v0.21.0
	// points to temporary-watch-reduction-patch-1.21 to pick up k/k/pull/101102 - please remove it once the pr merges and a new Z release is cut
	k8s.io/apiserver => github.com/openshift/kubernetes-apiserver v0.0.0-20210419140141-620426e63a99
	k8s.io/client-go => k8s.io/client-go v0.21.0
)
