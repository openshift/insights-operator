package clusterconfig

import (
	"context"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"sigs.k8s.io/yaml"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherConfigMaps fetches the ConfigMaps from namespace openshift-config
// and tries to fetch "cluster-monitoring-config" ConfigMap from openshift-monitoring namespace.
//
// Anonymization: If the content of ConfigMap contains a parseable PEM structure (like certificate) it removes the inside of PEM blocks.
// For ConfigMap of type BinaryData it is encoded as standard base64.
// In the archive under configmaps we store name of the namespace, name of the ConfigMap and then each ConfigMap Key.
// For example config/configmaps/NAMESPACENAME/CONFIGMAPNAME/CONFIGMAPKEY1
//
// The Kubernetes api https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/configmap.go#L80
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#configmaplist-v1core
//
// * Location in archive: config/configmaps/{namespace-name}/{configmap-name}/
// * See: docs/insights-archive-sample/config/configmaps
// * Id in config: config_maps
// * Since versions:
//   * 4.3.25+
//   * 4.4.6+
//   * 4.5+
// * "cluster-monitoring-config" ConfigMap data since versions:
//   * 4.6.22+
//   * 4.7+
func (g *Gatherer) GatherConfigMaps(ctx context.Context) ([]record.Record, []error) {
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	records, errors := gatherConfigMaps(ctx, gatherKubeClient.CoreV1())
	monitoringRec, monitoringErrs := gatherMonitoringCM(ctx, gatherKubeClient.CoreV1())
	records = append(records, monitoringRec...)
	errors = append(errors, monitoringErrs...)

	return records, errors
}

func gatherConfigMaps(ctx context.Context, coreClient corev1client.CoreV1Interface) ([]record.Record, []error) {
	cms, err := coreClient.ConfigMaps("openshift-config").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, []error{err}
	}
	records := make([]record.Record, 0, len(cms.Items))
	for i := range cms.Items {
		for dk, dv := range cms.Items[i].Data {
			records = append(records, record.Record{
				Name: fmt.Sprintf("config/configmaps/%s/%s/%s", cms.Items[i].Namespace, cms.Items[i].Name, dk),
				Item: ConfigMapAnonymizer{v: []byte(dv), encodeBase64: false},
			})
		}
		for dk, dv := range cms.Items[i].BinaryData {
			records = append(records, record.Record{
				Name: fmt.Sprintf("config/configmaps/%s/%s/%s", cms.Items[i].Namespace, cms.Items[i].Name, dk),
				Item: ConfigMapAnonymizer{v: dv, encodeBase64: true},
			})
		}
	}
	return records, nil
}

func gatherMonitoringCM(ctx context.Context, coreClient corev1client.CoreV1Interface) ([]record.Record, []error) {
	monitoringCM, err := coreClient.ConfigMaps("openshift-monitoring").Get(ctx, "cluster-monitoring-config", metav1.GetOptions{})
	if err != nil {
		return nil, []error{err}
	}
	records := make([]record.Record, 0)
	for dk, dv := range monitoringCM.Data {
		j, err := yaml.YAMLToJSON([]byte(dv))
		if err != nil {
			return nil, []error{err}
		}
		records = append(records, record.Record{
			Name: fmt.Sprintf("config/configmaps/%s/%s/%s", monitoringCM.Namespace, monitoringCM.Name, strings.TrimSuffix(dk, ".yaml")),
			Item: RawJSON(j),
		})
	}
	return records, nil
}

// ConfigMapAnonymizer implements serialization of configmap
// and potentially anonymizes if it is a certificate
type ConfigMapAnonymizer struct {
	v            []byte
	encodeBase64 bool
}

// Marshal implements serialization of Node with anonymization
func (a ConfigMapAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	c := []byte(anonymizeConfigMap(a.v))
	if a.encodeBase64 {
		buff := make([]byte, base64.StdEncoding.EncodedLen(len(c)))
		base64.StdEncoding.Encode(buff, c)
		c = buff
	}
	return c, nil
}

// GetExtension returns extension for anonymized openshift objects
func (a ConfigMapAnonymizer) GetExtension() string {
	return ""
}

func anonymizeConfigMap(dv []byte) string {
	anonymizedPemBlock := `-----BEGIN CERTIFICATE-----
ANONYMIZED
-----END CERTIFICATE-----
`
	var sb strings.Builder
	r := dv
	for {
		var block *pem.Block
		block, r = pem.Decode(r)
		if block == nil {
			// cannot be extracted
			return string(dv)
		}
		sb.WriteString(anonymizedPemBlock)
		if len(r) == 0 {
			break
		}
	}
	return sb.String()
}

type RawJSON []byte

// Marshal just returns bytes
func (r RawJSON) Marshal(_ context.Context) ([]byte, error) {
	return r, nil
}

// GetExtension returns json extension
func (r RawJSON) GetExtension() string {
	return jsonExtension
}
