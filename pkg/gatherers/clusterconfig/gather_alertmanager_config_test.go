package clusterconfig

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"

	"github.com/openshift/insights-operator/pkg/utils/anonymize"
)

func Test_gatherAlertmanagerConfig(t *testing.T) {
	tests := []struct {
		name            string
		secret          *corev1.Secret
		expectedRecords int
		expectedErrors  int
	}{
		{
			name:            "secret not found returns nil",
			secret:          nil,
			expectedRecords: 0,
			expectedErrors:  0,
		},
		{
			name: "missing alertmanager.yaml key returns nil",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: monitoringNamespace,
					Name:      "alertmanager-main",
				},
				Data: map[string][]byte{
					"some-other-key": []byte("value"),
				},
			},
			expectedRecords: 0,
			expectedErrors:  0,
		},
		{
			name: "malformed YAML returns error",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: monitoringNamespace,
					Name:      "alertmanager-main",
				},
				Data: map[string][]byte{
					"alertmanager.yaml": []byte("{{invalid yaml"),
				},
			},
			expectedRecords: 0,
			expectedErrors:  1,
		},
		{
			name: "default minimal config",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: monitoringNamespace,
					Name:      "alertmanager-main",
				},
				Data: map[string][]byte{
					"alertmanager.yaml": []byte(`
global:
  resolve_timeout: 5m
receivers:
  - name: "null"
route:
  group_by:
    - namespace
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 12h
  receiver: "null"
`),
				},
			},
			expectedRecords: 1,
			expectedErrors:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeClient := kubefake.NewClientset()
			if tt.secret != nil {
				_, err := kubeClient.CoreV1().Secrets(monitoringNamespace).Create(
					context.TODO(), tt.secret, metav1.CreateOptions{},
				)
				assert.NoError(t, err)
			}

			records, errs := gatherAlertmanagerConfig(context.Background(), kubeClient.CoreV1())

			if tt.expectedErrors > 0 {
				assert.Len(t, errs, tt.expectedErrors)
			} else {
				assert.Empty(t, errs)
			}
			assert.Len(t, records, tt.expectedRecords)
		})
	}
}

func Test_gatherAlertmanagerConfig_excludesGlobalSection(t *testing.T) {
	kubeClient := kubefake.NewClientset()
	_, err := kubeClient.CoreV1().Secrets(monitoringNamespace).Create(
		context.TODO(),
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: monitoringNamespace,
				Name:      "alertmanager-main",
			},
			Data: map[string][]byte{
				"alertmanager.yaml": []byte(`
global:
  resolve_timeout: 5m
  slack_api_url: https://hooks.slack.com/services/GLOBAL/SECRET/TOKEN
  smtp_smarthost: smtp.secret-server.com:587
receivers:
  - name: "null"
route:
  receiver: "null"
`),
			},
		},
		metav1.CreateOptions{},
	)
	assert.NoError(t, err)

	records, errs := gatherAlertmanagerConfig(context.Background(), kubeClient.CoreV1())
	assert.Empty(t, errs)
	assert.Len(t, records, 1)

	data, err := records[0].Item.Marshal()
	assert.NoError(t, err)

	assert.NotContains(t, string(data), "global")
	assert.NotContains(t, string(data), "slack.com")
	assert.NotContains(t, string(data), "smtp.secret-server.com")
}

func Test_gatherAlertmanagerConfig_fullConfig(t *testing.T) {
	kubeClient := kubefake.NewClientset()
	_, err := kubeClient.CoreV1().Secrets(monitoringNamespace).Create(
		context.TODO(),
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: monitoringNamespace,
				Name:      "alertmanager-main",
			},
			Data: map[string][]byte{
				"alertmanager.yaml": []byte(`
receivers:
  - name: slack-notifications
    slack_configs:
      - api_url: https://hooks.slack.com/services/T00/B00/XXXX
        channel: "#alerts"
        send_resolved: true
  - name: pagerduty-critical
    pagerduty_configs:
      - service_key: abcdef1234567890
        routing_key: R0123456789ABCDEF
  - name: webhook-receiver
    webhook_configs:
      - url: https://webhook.example.com/alert?token=secret123
        max_alerts: 10
  - name: email-team
    email_configs:
      - to: team@example.com
        smarthost: smtp.example.com:587
        auth_username: alertuser
        auth_password: supersecret
route:
  receiver: slack-notifications
  group_by:
    - namespace
  routes:
    - receiver: pagerduty-critical
      matchers:
        - severity="critical"
inhibit_rules:
  - source_matchers:
      - severity="critical"
    target_matchers:
      - severity="warning"
    equal:
      - namespace
`),
			},
		},
		metav1.CreateOptions{},
	)
	assert.NoError(t, err)

	records, errs := gatherAlertmanagerConfig(context.Background(), kubeClient.CoreV1())
	assert.Empty(t, errs)
	assert.Len(t, records, 1)
	assert.Equal(t, "config/secrets/openshift-monitoring/alertmanager-main/data", records[0].Name)

	data, err := records[0].Item.Marshal()
	assert.NoError(t, err)

	// Verify non-sensitive fields are preserved
	assert.Contains(t, string(data), `"slack-notifications"`)
	assert.Contains(t, string(data), `"pagerduty-critical"`)
	assert.Contains(t, string(data), `"webhook-receiver"`)
	assert.Contains(t, string(data), `"email-team"`)
	assert.Contains(t, string(data), `"#alerts"`)
	assert.Contains(t, string(data), `"team@example.com"`)

	// Verify sensitive fields are anonymized
	assert.NotContains(t, string(data), "hooks.slack.com")
	assert.NotContains(t, string(data), "abcdef1234567890")
	assert.NotContains(t, string(data), "R0123456789ABCDEF")
	assert.NotContains(t, string(data), "webhook.example.com")
	assert.NotContains(t, string(data), "supersecret")
	assert.NotContains(t, string(data), "alertuser")
	assert.NotContains(t, string(data), "smtp.example.com")

	// Verify route structure is preserved
	assert.Contains(t, string(data), `"namespace"`)
	assert.Contains(t, string(data), `severity=`)

	// Verify inhibit rules are preserved
	assert.Contains(t, string(data), `"source_matchers"`)
	assert.Contains(t, string(data), `"target_matchers"`)
}

func Test_anonymizeMap(t *testing.T) {
	m := map[string]interface{}{
		"receivers": []interface{}{
			map[string]interface{}{
				"name": "test-receiver",
				"webhook_configs": []interface{}{
					map[string]interface{}{
						"url":        "https://example.com/webhook",
						"max_alerts": 5,
					},
				},
				"email_configs": []interface{}{
					map[string]interface{}{
						"to":            "oncall@example.com",
						"smarthost":     "smtp.example.com:587",
						"auth_password": "pass",
						"send_resolved": true,
					},
				},
				"slack_configs": []interface{}{
					map[string]interface{}{
						"api_url": "https://hooks.slack.com/services/T00/B00/XXXX",
						"channel": "#alerts",
					},
				},
			},
		},
		"route": map[string]interface{}{
			"receiver": "test-receiver",
			"group_by": []interface{}{"namespace"},
		},
		"inhibit_rules": []interface{}{
			map[string]interface{}{
				"source_matchers": []interface{}{`severity="critical"`},
				"target_matchers": []interface{}{`severity="warning"`},
				"equal":           []interface{}{"namespace"},
			},
		},
	}

	anonymizeMap(m, false)

	receivers := m["receivers"].([]interface{})
	recv := receivers[0].(map[string]interface{})

	// Sensitive keys anonymized
	whCfg := recv["webhook_configs"].([]interface{})[0].(map[string]interface{})
	assert.Equal(t, anonymize.String("https://example.com/webhook"), whCfg["url"])
	assert.Equal(t, 5, whCfg["max_alerts"])

	emailCfg := recv["email_configs"].([]interface{})[0].(map[string]interface{})
	assert.Equal(t, "oncall@example.com", emailCfg["to"])
	assert.Equal(t, anonymize.String("smtp.example.com:587"), emailCfg["smarthost"])
	assert.Equal(t, anonymize.String("pass"), emailCfg["auth_password"])
	assert.Equal(t, true, emailCfg["send_resolved"])

	slackCfg := recv["slack_configs"].([]interface{})[0].(map[string]interface{})
	assert.Equal(t, anonymize.String("https://hooks.slack.com/services/T00/B00/XXXX"), slackCfg["api_url"])
	assert.Equal(t, "#alerts", slackCfg["channel"])

	// Receiver name preserved
	assert.Equal(t, "test-receiver", recv["name"])

	// Route and inhibit rules unchanged
	route := m["route"].(map[string]interface{})
	assert.Equal(t, "test-receiver", route["receiver"])
	assert.Equal(t, []interface{}{"namespace"}, route["group_by"])

	rules := m["inhibit_rules"].([]interface{})
	rule := rules[0].(map[string]interface{})
	assert.Equal(t, []interface{}{`severity="critical"`}, rule["source_matchers"])
	assert.Equal(t, []interface{}{`severity="warning"`}, rule["target_matchers"])
}

func Test_anonymizeMap_nestedCredentials(t *testing.T) {
	m := map[string]interface{}{
		"receivers": []interface{}{
			map[string]interface{}{
				"name": "bearer-receiver",
				"webhook_configs": []interface{}{
					map[string]interface{}{
						"http_config": map[string]interface{}{
							"authorization": map[string]interface{}{
								"type":        "Bearer",
								"credentials": "my-bearer-token",
							},
							"basic_auth": map[string]interface{}{
								"username": "admin",
								"password": "s3cret",
							},
						},
						"url": "https://webhook.example.com/alert",
					},
				},
			},
		},
		"route": map[string]interface{}{
			"receiver": "bearer-receiver",
			"routes": []interface{}{
				map[string]interface{}{
					"receiver": "bearer-receiver",
					"matchers": []interface{}{`severity="critical"`},
				},
			},
		},
	}

	anonymizeMap(m, false)

	receivers := m["receivers"].([]interface{})
	whCfgs := receivers[0].(map[string]interface{})["webhook_configs"].([]interface{})
	httpCfg := whCfgs[0].(map[string]interface{})["http_config"].(map[string]interface{})

	// authorization.credentials anonymized via parent "authorization" matching "auth"
	authz := httpCfg["authorization"].(map[string]interface{})
	assert.Equal(t, anonymize.String("Bearer"), authz["type"])
	assert.Equal(t, anonymize.String("my-bearer-token"), authz["credentials"])

	// basic_auth children anonymized via parent "basic_auth" matching "auth"
	basicAuth := httpCfg["basic_auth"].(map[string]interface{})
	assert.Equal(t, anonymize.String("admin"), basicAuth["username"])
	assert.Equal(t, anonymize.String("s3cret"), basicAuth["password"])

	// url is anonymized by its own key matching the pattern
	assert.Equal(t, anonymize.String("https://webhook.example.com/alert"), whCfgs[0].(map[string]interface{})["url"])

	// route structure is preserved (non-sensitive nested maps)
	route := m["route"].(map[string]interface{})
	assert.Equal(t, "bearer-receiver", route["receiver"])
	routes := route["routes"].([]interface{})
	subRoute := routes[0].(map[string]interface{})
	assert.Equal(t, "bearer-receiver", subRoute["receiver"])
	assert.Equal(t, []interface{}{`severity="critical"`}, subRoute["matchers"])
}
