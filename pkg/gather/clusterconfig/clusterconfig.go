package clusterconfig

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/client-go/config/clientset/versioned/scheme"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

var serializer = scheme.Codecs.LegacyCodec(configv1.GroupVersion)

type Gatherer struct {
	client configv1client.ConfigV1Interface

	lock        sync.Mutex
	lastVersion *configv1.ClusterVersion
}

func New(client configv1client.ConfigV1Interface) *Gatherer {
	return &Gatherer{
		client: client,
	}
}

func (i *Gatherer) Gather(ctx context.Context, recorder record.Interface) error {
	return records(recorder,
		func() ([]record.Record, []error) {
			config, err := i.client.ClusterOperators().List(metav1.ListOptions{})
			if err != nil {
				return nil, []error{err}
			}
			records := make([]record.Record, 0, len(config.Items))
			for i := range config.Items {
				records = append(records, record.Record{Name: fmt.Sprintf("config/clusteroperator/%s", config.Items[i].Name), Item: ClusterOperatorAnonymizer{&config.Items[i]}})
			}
			return records, nil
		},
		func() (record.Record, error) {
			config, err := i.client.ClusterVersions().Get("version", metav1.GetOptions{})
			if err != nil {
				return record.Record{}, err
			}
			i.setClusterVersion(config)
			return record.Record{Name: "config/version", Item: ClusterVersionAnonymizer{config}}, nil
		},
		func() (record.Record, error) {
			version := i.ClusterVersion()
			if version == nil {
				return record.Record{}, errSkipRecord
			}
			return record.Record{Name: "config/id", Item: Raw{string(version.Spec.ClusterID)}}, nil
		},
		func() (record.Record, error) {
			config, err := i.client.Infrastructures().Get("cluster", metav1.GetOptions{})
			if err != nil {
				return record.Record{}, err
			}
			return record.Record{Name: "config/infrastructure", Item: InfrastructureAnonymizer{config}}, nil
		},
		func() (record.Record, error) {
			config, err := i.client.Networks().Get("cluster", metav1.GetOptions{})
			if err != nil {
				return record.Record{}, err
			}
			return record.Record{Name: "config/network", Item: Anonymizer{config}}, nil
		},
		func() (record.Record, error) {
			config, err := i.client.Authentications().Get("cluster", metav1.GetOptions{})
			if err != nil {
				return record.Record{}, err
			}
			return record.Record{Name: "config/authentication", Item: Anonymizer{config}}, nil
		},
		func() (record.Record, error) {
			config, err := i.client.FeatureGates().Get("cluster", metav1.GetOptions{})
			if err != nil {
				return record.Record{}, err
			}
			return record.Record{Name: "config/featuregate", Item: FeatureGateAnonymizer{config}}, nil
		},
		func() (record.Record, error) {
			config, err := i.client.OAuths().Get("cluster", metav1.GetOptions{})
			if err != nil {
				return record.Record{}, err
			}
			return record.Record{Name: "config/oauth", Item: Anonymizer{config}}, nil
		},
		func() (record.Record, error) {
			config, err := i.client.Ingresses().Get("cluster", metav1.GetOptions{})
			if err != nil {
				return record.Record{}, err
			}
			return record.Record{Name: "config/ingress", Item: IngressAnonymizer{config}}, nil
		},
	)
}

var errSkipRecord = fmt.Errorf("skip recording")

func records(recorder record.Interface, bulkFn func() ([]record.Record, []error), fns ...func() (record.Record, error)) error {
	var errors []string
	if bulkFn != nil {
		records, errs := bulkFn()
		for _, err := range errs {
			errors = append(errors, err.Error())
		}
		for _, r := range records {
			if err := recorder.Record(r); err != nil {
				errors = append(errors, fmt.Sprintf("unable to record %s: %v", r.Name, err.Error()))
				continue
			}
		}
	}
	for _, fn := range fns {
		r, err := fn()
		if err != nil {
			if err != errSkipRecord {
				errors = append(errors, err.Error())
			}
			continue
		}
		if err := recorder.Record(r); err != nil {
			errors = append(errors, fmt.Sprintf("unable to record %s: %v", r.Name, err.Error()))
			continue
		}
	}
	if len(errors) > 0 {
		sort.Strings(errors)
		errors = uniqueStrings(errors)
		return fmt.Errorf("failed to gather cluster config: %s", strings.Join(errors, ", "))
	}
	return nil
}

func uniqueStrings(arr []string) []string {
	var last int
	for i := 1; i < len(arr); i++ {
		if arr[i] == arr[last] {
			continue
		}
		last++
		if last != i {
			arr[last] = arr[i]
		}
	}
	if last < len(arr) {
		last++
	}
	return arr[:last]
}

type Raw struct{ string }

func (r Raw) Marshal(_ context.Context) ([]byte, error) {
	return []byte(r.string), nil
}

type Anonymizer struct{ runtime.Object }

func (a Anonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(serializer, a.Object)
}

type InfrastructureAnonymizer struct{ *configv1.Infrastructure }

func (a InfrastructureAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(serializer, anonymizeInfrastructure(a.Infrastructure))
}

func anonymizeInfrastructure(config *configv1.Infrastructure) *configv1.Infrastructure {
	config.Status.APIServerURL = anonymizeURL(config.Status.APIServerURL)
	config.Status.EtcdDiscoveryDomain = anonymizeURL(config.Status.EtcdDiscoveryDomain)
	config.Status.InfrastructureName = anonymizeURL(config.Status.InfrastructureName)
	return config
}

type ClusterVersionAnonymizer struct{ *configv1.ClusterVersion }

func (a ClusterVersionAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	a.ClusterVersion.Spec.Upstream = configv1.URL(anonymizeURL(string(a.ClusterVersion.Spec.Upstream)))
	return runtime.Encode(serializer, a.ClusterVersion)
}

type FeatureGateAnonymizer struct{ *configv1.FeatureGate }

func (a FeatureGateAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(serializer, a.FeatureGate)
}

type IngressAnonymizer struct{ *configv1.Ingress }

func (a IngressAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	a.Ingress.Spec.Domain = anonymizeURL(a.Ingress.Spec.Domain)
	return runtime.Encode(serializer, a.Ingress)
}

var reURL = regexp.MustCompile(`[^.\-/:]`)

func anonymizeURL(s string) string { return reURL.ReplaceAllString(s, "x") }

type ClusterOperatorAnonymizer struct{ *configv1.ClusterOperator }

func (a ClusterOperatorAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(serializer, a.ClusterOperator)
}

// type ClusterOperatorListAnonymizer struct{ *configv1.ClusterOperatorList }

// func (a ClusterOperatorListAnonymizer) Marshal(_ context.Context) ([]byte, error) {
// 	return runtime.Encode(serializer, a.ClusterOperatorList)
// }

// func clusterOperatorListResourceVersion(items *configv1.ClusterOperatorList) string {
// 	rvs := make([]string, 0, len(items.Items))
// 	for _, item := range items.Items {
// 		rvs = append(rvs, item.ResourceVersion)
// 	}
// 	sort.Strings(rvs)
// 	return strings.Join(rvs, ",")
// }

func (i *Gatherer) setClusterVersion(version *configv1.ClusterVersion) {
	i.lock.Lock()
	defer i.lock.Unlock()
	if i.lastVersion != nil && i.lastVersion.ResourceVersion == version.ResourceVersion {
		return
	}
	i.lastVersion = version.DeepCopy()
}

func (i *Gatherer) ClusterVersion() *configv1.ClusterVersion {
	i.lock.Lock()
	defer i.lock.Unlock()
	return i.lastVersion
}
