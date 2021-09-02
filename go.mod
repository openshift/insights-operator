module github.com/openshift/insights-operator

go 1.16

require (
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/openshift/api v0.0.0-20210901140736-d8ed1449662d
	github.com/openshift/client-go v0.0.0-20210521082421-73d9475a9142
	github.com/openshift/installer v0.9.0-master.0.20191219195746-103098955ced
	github.com/openshift/library-go v0.0.0-20210521084623-7392ea9b02ca
	github.com/prometheus/common v0.26.0
	github.com/spf13/cobra v1.1.3
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonschema v1.2.0
	golang.org/x/net v0.0.0-20210520170846-37e1c6afe023
	golang.org/x/time v0.0.0-20210723032227-1f47c861a9ac
	k8s.io/api v0.22.1
	k8s.io/apiextensions-apiserver v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/component-base v0.22.1
	k8s.io/klog/v2 v2.9.0
	k8s.io/utils v0.0.0-20210707171843-4b05e18ac7d9
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/openshift/api => github.com/openshift/api v0.0.0-20210901140736-d8ed1449662d
	github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20210409155308-a8e62c60e930
	github.com/openshift/library-go => github.com/openshift/library-go v0.0.0-20210521084623-7392ea9b02ca
	k8s.io/api => k8s.io/api v0.22.1
	k8s.io/apimachinery => k8s.io/apimachinery v0.22.1
	k8s.io/client-go => k8s.io/client-go v0.22.1
)
