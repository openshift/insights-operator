package clusterconfig

import (
	"context"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	_ "k8s.io/apimachinery/pkg/runtime/serializer/yaml"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherConfigMaps fetches the ConfigMaps from namespace openshift-config.
//
// Anonymization: If the content of ConfigMap contains a parseable PEM structure (like certificate) it removes the inside of PEM blocks.
// For ConfigMap of type BinaryData it is encoded as standard base64.
// In the archive under configmaps we store name of ConfigMap and then each ConfigMap Key. For example config/configmaps/CONFIGMAPNAME/CONFIGMAPKEY1
//
// The Kubernetes api https://github.com/kubernetes/client-go/blob/master/kubernetes/typed/core/v1/configmap.go#L80
// Response see https://docs.openshift.com/container-platform/4.3/rest_api/index.html#configmaplist-v1core
//
// Location in archive: config/configmaps/
// See: docs/insights-archive-sample/config/configmaps
func GatherConfigMaps(g *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		cms, err := g.coreClient.ConfigMaps("openshift-config").List(g.ctx, metav1.ListOptions{})
		if err != nil {
			return nil, []error{err}
		}
		records := make([]record.Record, 0, len(cms.Items))
		for i := range cms.Items {
			for dk, dv := range cms.Items[i].Data {
				records = append(records, record.Record{
					Name: fmt.Sprintf("config/configmaps/%s/%s", cms.Items[i].Name, dk),
					Item: ConfigMapAnonymizer{v: []byte(dv), encodeBase64: false},
				})
			}
			for dk, dv := range cms.Items[i].BinaryData {
				records = append(records, record.Record{
					Name: fmt.Sprintf("config/configmaps/%s/%s", cms.Items[i].Name, dk),
					Item: ConfigMapAnonymizer{v: dv, encodeBase64: true},
				})
			}
		}

		return records, nil
	}
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
		base64.StdEncoding.Encode(buff, []byte(c))
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
