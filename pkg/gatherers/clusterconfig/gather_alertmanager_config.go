package clusterconfig

import (
	"context"
	"regexp"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/anonymize"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"sigs.k8s.io/yaml"
)

var sensitiveFieldPattern = regexp.MustCompile(`(?i)(token|key|secret|password|auth|url|host)`)

// GatherAlertmanagerConfig Collects the anonymized Alertmanager routing
// configuration from the alertmanager-main secret in the openshift-monitoring
// namespace. Only receivers, route, and inhibit_rules are extracted.
// Sensitive fields (webhook URLs, API keys, passwords, tokens) are anonymized.
//
// ### API Reference
// None
//
// ### Sample data
// - docs/insights-archive-sample/config/secrets/openshift-monitoring/alertmanager-main/data.json
//
// ### Location in archive
// - `config/secrets/openshift-monitoring/alertmanager-main/data.json`
//
// ### Config ID
// `clusterconfig/alertmanager_config`
//
// ### Released version
// - 5.0.0
//
// ### Backported versions
// None
//
// ### Changes
// None
func (g *Gatherer) GatherAlertmanagerConfig(ctx context.Context) ([]record.Record, []error) {
	gatherKubeClient, err := kubernetes.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return gatherAlertmanagerConfig(ctx, gatherKubeClient.CoreV1())
}

func gatherAlertmanagerConfig(ctx context.Context, cli v1.CoreV1Interface) ([]record.Record, []error) {
	secret, err := cli.Secrets(monitoringNamespace).Get(ctx, "alertmanager-main", metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{err}
	}

	yamlData, found := secret.Data["alertmanager.yaml"]
	if !found {
		return nil, nil
	}

	var fullCfg map[string]interface{}
	if err := yaml.Unmarshal(yamlData, &fullCfg); err != nil {
		return nil, []error{err}
	}

	cfg := make(map[string]interface{})
	for _, key := range []string{"receivers", "route", "inhibit_rules"} {
		if val, ok := fullCfg[key]; ok {
			cfg[key] = val
		}
	}

	anonymizeMap(cfg, false)

	return []record.Record{{
		Name: "config/secrets/openshift-monitoring/alertmanager-main/data",
		Item: record.JSONMarshaller{Object: cfg},
	}}, nil
}

// anonymizeMap redacts string values in m whose key matches sensitiveFieldPattern.
// Sensitivity propagates to all descendants so that nested fields like
// authorization.credentials are redacted even when "credentials" alone
// doesn't match the pattern — the parent "authorization" matches "auth".
func anonymizeMap(m map[string]interface{}, sensitive bool) {
	for key, val := range m {
		isSensitive := sensitive || sensitiveFieldPattern.MatchString(key)
		switch v := val.(type) {
		case string:
			if isSensitive {
				m[key] = anonymize.String(v)
			}
		case map[string]interface{}:
			anonymizeMap(v, isSensitive)
		case []interface{}:
			anonymizeSlice(v, isSensitive)
		}
	}
}

func anonymizeSlice(s []interface{}, sensitive bool) {
	for i, item := range s {
		switch v := item.(type) {
		case map[string]interface{}:
			anonymizeMap(v, sensitive)
		case string:
			if sensitive {
				s[i] = anonymize.String(v)
			}
		}
	}
}
