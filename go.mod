module github.com/openshift/insights-operator

go 1.16

require (
	github.com/blang/semver/v4 v4.0.0
	github.com/evanphx/json-patch v4.12.0+incompatible
	github.com/go-logr/logr v1.2.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da
	github.com/google/go-cmp v0.5.6 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/openshift/api v0.0.0-20220525145417-ee5b62754c68
	github.com/openshift/build-machinery-go v0.0.0-20211213093930-7e33a7eb4ce3
	github.com/openshift/client-go v0.0.0-20220525160904-9e1acff93e4a
	github.com/openshift/installer v0.9.0-master.0.20191219195746-103098955ced
	github.com/openshift/library-go v0.0.0-20220525173854-9b950a41acdc
	github.com/prometheus/common v0.32.1
	github.com/spf13/cobra v1.4.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonschema v1.2.0
	golang.org/x/net v0.0.0-20220127200216-cd36cc0744dd
	golang.org/x/time v0.0.0-20220210224613-90d013bbcef8
	k8s.io/api v0.24.0
	k8s.io/apiextensions-apiserver v0.24.0
	k8s.io/apimachinery v0.24.0
	k8s.io/client-go v0.24.0
	k8s.io/component-base v0.24.0
	k8s.io/klog/v2 v2.60.1
	k8s.io/utils v0.0.0-20220210201930-3a6ce19ff2f9
	sigs.k8s.io/yaml v1.2.0
)
