module github.com/openshift/insights-operator

go 1.16

require (
	github.com/blang/semver/v4 v4.0.0
	github.com/go-logr/logr v1.2.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da
	github.com/google/go-cmp v0.5.6 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/openshift/api v0.0.0-20220110170128-ab6fffcd6b76
	github.com/openshift/client-go v0.0.0-20211209144617-7385dd6338e3
	github.com/openshift/installer v0.9.0-master.0.20191219195746-103098955ced
	github.com/openshift/library-go v0.0.0-20220110124235-d70e915b42bd
	github.com/prometheus/common v0.28.0
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonschema v1.2.0
	golang.org/x/net v0.0.0-20220111093109-d55c255bac03
	golang.org/x/time v0.0.0-20210723032227-1f47c861a9ac
	k8s.io/api v0.23.1
	k8s.io/apiextensions-apiserver v0.23.0
	k8s.io/apimachinery v0.23.1
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/component-base v0.23.0
	k8s.io/klog/v2 v2.40.1
	k8s.io/utils v0.0.0-20210930125809-cb0fa318a74b
	sigs.k8s.io/yaml v1.2.0
)

replace k8s.io/client-go => k8s.io/client-go v0.23.1
