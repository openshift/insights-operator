package clusterconfig

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/client-go/config/clientset/versioned/scheme"
	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

var (
	serializer     = scheme.Codecs.LegacyCodec(configv1.SchemeGroupVersion)
	kubeSerializer = kubescheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion)
)

type Gatherer struct {
	client     configv1client.ConfigV1Interface
	coreClient corev1client.CoreV1Interface

	lock        sync.Mutex
	lastVersion *configv1.ClusterVersion
}

func New(client configv1client.ConfigV1Interface, coreClient corev1client.CoreV1Interface) *Gatherer {
	return &Gatherer{
		client:     client,
		coreClient: coreClient,
	}
}

var reInvalidUIDCharacter = regexp.MustCompile(`[^a-z0-9\-]`)

func (i *Gatherer) Gather(ctx context.Context, recorder record.Interface) error {
	return record.Collect(ctx, recorder,
		record.Aggregate(
			func() ([]record.Record, []error) {
				config, err := i.client.ClusterOperators().List(metav1.ListOptions{})
				if errors.IsNotFound(err) {
					return nil, nil
				}
				if err != nil {
					return nil, []error{err}
				}
				records := make([]record.Record, 0, len(config.Items))
				for i := range config.Items {
					records = append(records, record.Record{Name: fmt.Sprintf("config/clusteroperator/%s", config.Items[i].Name), Item: ClusterOperatorAnonymizer{&config.Items[i]}})
				}

				return records, nil
			},
			func() ([]record.Record, []error) {
				nodes, err := i.coreClient.Nodes().List(metav1.ListOptions{})
				if err != nil {
					return nil, []error{err}
				}
				records := make([]record.Record, 0, len(nodes.Items))
				for i := range nodes.Items {
					if isHealthyNode(&nodes.Items[i]) {
						continue
					}
					records = append(records, record.Record{Name: fmt.Sprintf("config/node/%s", nodes.Items[i].Name), Item: NodeAnonymizer{&nodes.Items[i]}})
				}

				return records, nil
			},
		),
		func() (record.Record, error) {
			config, err := i.client.ClusterVersions().Get("version", metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return record.Record{}, record.ErrSkipRecord
			}
			if err != nil {
				return record.Record{}, err
			}
			i.setClusterVersion(config)
			return record.Record{Name: "config/version", Item: ClusterVersionAnonymizer{config}}, nil
		},
		func() (record.Record, error) {
			version := i.ClusterVersion()
			if version == nil {
				return record.Record{}, record.ErrSkipRecord
			}
			return record.Record{Name: "config/id", Item: Raw{string(version.Spec.ClusterID)}}, nil
		},
		func() (record.Record, error) {
			config, err := i.client.Infrastructures().Get("cluster", metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return record.Record{}, record.ErrSkipRecord
			}
			if err != nil {
				return record.Record{}, err
			}
			return record.Record{Name: "config/infrastructure", Item: InfrastructureAnonymizer{config}}, nil
		},
		func() (record.Record, error) {
			config, err := i.client.Networks().Get("cluster", metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return record.Record{}, record.ErrSkipRecord
			}
			if err != nil {
				return record.Record{}, err
			}
			return record.Record{Name: "config/network", Item: Anonymizer{config}}, nil
		},
		func() (record.Record, error) {
			config, err := i.client.Authentications().Get("cluster", metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return record.Record{}, record.ErrSkipRecord
			}
			if err != nil {
				return record.Record{}, err
			}
			return record.Record{Name: "config/authentication", Item: Anonymizer{config}}, nil
		},
		func() (record.Record, error) {
			config, err := i.client.FeatureGates().Get("cluster", metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return record.Record{}, record.ErrSkipRecord
			}
			if err != nil {
				return record.Record{}, err
			}
			return record.Record{Name: "config/featuregate", Item: FeatureGateAnonymizer{config}}, nil
		},
		func() (record.Record, error) {
			config, err := i.client.OAuths().Get("cluster", metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return record.Record{}, record.ErrSkipRecord
			}
			if err != nil {
				return record.Record{}, err
			}
			return record.Record{Name: "config/oauth", Item: Anonymizer{config}}, nil
		},
		func() (record.Record, error) {
			config, err := i.client.Ingresses().Get("cluster", metav1.GetOptions{})
			if errors.IsNotFound(err) {
				return record.Record{}, record.ErrSkipRecord
			}
			if err != nil {
				return record.Record{}, err
			}
			return record.Record{Name: "config/ingress", Item: IngressAnonymizer{config}}, nil
		},
	)
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
	config.Status.APIServerInternalURL = anonymizeURL(config.Status.APIServerInternalURL)
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

var reURL = regexp.MustCompile(`[^\.\-/\:]`)

func anonymizeURL(s string) string { return reURL.ReplaceAllString(s, "x") }

type ClusterOperatorAnonymizer struct{ *configv1.ClusterOperator }

func (a ClusterOperatorAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(serializer, a.ClusterOperator)
}

type NodeAnonymizer struct{ *corev1.Node }

func (a NodeAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(kubeSerializer, anonymizeNode(a.Node))
}

func anonymizeNode(node *corev1.Node) *corev1.Node {
	for k := range node.Annotations {
		if isProductNamespacedKey(k) {
			continue
		}
		node.Annotations[k] = ""
	}
	for k, v := range node.Labels {
		if isProductNamespacedKey(k) {
			continue
		}
		node.Labels[k] = anonymizeString(v)
	}
	for i := range node.Status.Addresses {
		node.Status.Addresses[i].Address = anonymizeURL(node.Status.Addresses[i].Address)
	}
	node.Status.NodeInfo.BootID = anonymizeString(node.Status.NodeInfo.BootID)
	node.Status.NodeInfo.SystemUUID = anonymizeString(node.Status.NodeInfo.SystemUUID)
	node.Status.NodeInfo.MachineID = anonymizeString(node.Status.NodeInfo.MachineID)
	node.Status.Images = nil
	return node
}

func anonymizeString(s string) string {
	return strings.Repeat("x", len(s))
}

func isProductNamespacedKey(key string) bool {
	return strings.Contains(key, "openshift.io/") || strings.Contains(key, "k8s.io/") || strings.Contains(key, "kubernetes.io/")
}

func isHealthyNode(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady && condition.Status != corev1.ConditionTrue {
			return false
		}
	}
	return true
}

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
