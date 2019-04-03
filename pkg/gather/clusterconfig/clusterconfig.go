package clusterconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	configv1 "github.com/openshift/api/config/v1"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"

	"github.com/openshift/support-operator/pkg/record"
)

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
		func() (record.Record, error) {
			config, err := i.client.ClusterVersions().Get("version", metav1.GetOptions{})
			if err != nil {
				return record.Record{}, err
			}
			i.setClusterVersion(config)
			return record.Record{Name: "config/version", Fingerprint: config.ResourceVersion, Item: ClusterVersionAnonymizer{config}}, nil
		},
		func() (record.Record, error) {
			config, err := i.client.Infrastructures().Get("cluster", metav1.GetOptions{})
			if err != nil {
				return record.Record{}, err
			}
			return record.Record{Name: "config/infrastructure", Fingerprint: config.ResourceVersion, Item: InfrastructureAnonymizer{config}}, nil
		},
		func() (record.Record, error) {
			config, err := i.client.Networks().Get("cluster", metav1.GetOptions{})
			if err != nil {
				return record.Record{}, err
			}
			return record.Record{Name: "config/network", Fingerprint: config.ResourceVersion, Item: Anonymizer{config}}, nil
		},
		func() (record.Record, error) {
			config, err := i.client.Authentications().Get("cluster", metav1.GetOptions{})
			if err != nil {
				return record.Record{}, err
			}
			return record.Record{Name: "config/authentication", Fingerprint: config.ResourceVersion, Item: Anonymizer{config}}, nil
		},
		func() (record.Record, error) {
			config, err := i.client.OAuths().Get("cluster", metav1.GetOptions{})
			if err != nil {
				return record.Record{}, err
			}
			return record.Record{Name: "config/oauth", Fingerprint: config.ResourceVersion, Item: Anonymizer{config}}, nil
		},
		func() (record.Record, error) {
			config, err := i.client.Ingresses().Get("cluster", metav1.GetOptions{})
			if err != nil {
				return record.Record{}, err
			}
			return record.Record{Name: "config/ingress", Fingerprint: config.ResourceVersion, Item: IngressAnonymizer{config}}, nil
		},
	)
}

func records(recorder record.Interface, fns ...func() (record.Record, error)) error {
	var errors []string
	for _, fn := range fns {
		record, err := fn()
		if err != nil {
			errors = append(errors, err.Error())
			continue
		}
		if err := recorder.Record(record); err != nil {
			errors = append(errors, fmt.Sprintf("unable to record %s: %v", record.Name, err.Error()))
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

type Anonymizer struct{ runtime.Object }

func (a Anonymizer) Marshal(_ context.Context) ([]byte, error) {
	return json.Marshal(a.Object)
}

type InfrastructureAnonymizer struct{ *configv1.Infrastructure }

func (a InfrastructureAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return json.Marshal(anonymizeInfrastructure(a.Infrastructure))
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
	return json.Marshal(a.ClusterVersion)
}

type IngressAnonymizer struct{ *configv1.Ingress }

func (a IngressAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	a.Ingress.Spec.Domain = anonymizeURL(a.Ingress.Spec.Domain)
	return json.Marshal(a.Ingress)
}

var reURL = regexp.MustCompile(`[^\.\-/\:]`)

func anonymizeURL(s string) string { return reURL.ReplaceAllString(s, "x") }

// type IngressListAnonymizer struct{ *configv1.IngressList }

// func (a IngressListAnonymizer) Marshal(_ context.Context) ([]byte, error) {
// 	for i := range a.IngressList.Items {
// 		a.IngressList.Items[i].Spec.Domain = anonymizeURL(a.IngressList.Items[i].Spec.Domain)
// 	}
// 	return json.Marshal(a.IngressList)
// }

// func ingressListResourceVersion(items *configv1.IngressList) string {
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
