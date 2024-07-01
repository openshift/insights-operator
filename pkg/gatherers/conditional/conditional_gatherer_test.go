package conditional

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest/fake"

	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/gatherers"
)

func Test_Gatherer_Basic(t *testing.T) {
	gatherer := newEmptyGatherer("", "")

	assert.Equal(t, "conditional", gatherer.GetName())
	gatheringFunctions, err := gatherer.GetGatheringFunctions(context.Background())
	assert.NoError(t, err)
	assert.Len(t, gatheringFunctions, 2)

	assert.Implements(t, (*gatherers.Interface)(nil), gatherer)
	var g interface{} = gatherer
	_, ok := g.(gatherers.CustomPeriodGatherer)
	assert.False(t, ok, "should NOT implement gather.CustomPeriodGatherer")
}

func Test_Gatherer_GetGatheringFunctions(t *testing.T) {
	gatherer := newEmptyGatherer("", "")

	ctx := context.Background()
	err := gatherer.updateAlertsCache(ctx, newFakeClientWithAlerts("SamplesImagestreamImportFailing"))
	assert.NoError(t, err)

	gatheringFunctions, err := gatherer.GetGatheringFunctions(ctx)
	assert.NoError(t, err)
	assert.Len(t, gatheringFunctions, 4)
	_, found := gatheringFunctions["conditional_gatherer_rules"]
	assert.True(t, found)
}

func Test_Gatherer_GetGatheringFunctions_BuiltInConfigIsUsed(t *testing.T) {
	gatherer := newEmptyGatherer("", "")

	ctx := context.Background()
	err := gatherer.updateAlertsCache(ctx, newFakeClientWithAlerts("SamplesImagestreamImportFailing"))
	assert.NoError(t, err)

	gatheringFunctions, err := gatherer.GetGatheringFunctions(ctx)
	assert.NoError(t, err)
	assert.Len(t, gatheringFunctions, 4)
	_, found := gatheringFunctions["conditional_gatherer_rules"]
	assert.True(t, found)

	// the service suddenly died

	gatherer.insightsCli = &MockGatheringRulesServiceClient{
		err: fmt.Errorf("404"),
	}

	// but we still expect the same rules (from the cache)
	gatheringFunctions, err = gatherer.GetGatheringFunctions(ctx)
	assert.EqualError(t, err, "404")
	assert.Len(t, gatheringFunctions, 4)
	_, found = gatheringFunctions["conditional_gatherer_rules"]
	assert.True(t, found)
}

func Test_Gatherer_GetGatheringFunctions_InvalidConfig(t *testing.T) {
	gathererConfig := `{
		"version": "1.0.0",
		"conditional_gathering_rules": [{
			"conditions": [{
				"type": "alert_is_firing",
				"alert": {
					"name": "SamplesImagestreamImportFailing"
				}
			}],
			"gathering_functions": {
				"logs_of_namespace": {
					"namespace": "not-openshift-cluster-samples-operator",
					"tail_lines": 100
				}
			}
		}]
	}` // invalid namespace (doesn't start with openshift-)

	gatherer := newEmptyGatherer(gathererConfig, "")

	err := gatherer.updateAlertsCache(context.TODO(), newFakeClientWithAlerts("SamplesImagestreamImportFailing"))
	assert.NoError(t, err)

	gatheringFunctions, err := gatherer.GetGatheringFunctions(context.TODO())
	assert.EqualError(
		t,
		err,
		"got invalid config for conditional gatherer: 0.gathering_functions.logs_of_namespace.namespace: "+
			"Does not match pattern '^openshift-[a-zA-Z0-9_.-]{1,128}$'",
	)
	assert.Empty(t, gatheringFunctions)
}

func Test_Gatherer_GetGatheringFunctions_NoConditionsAreSatisfied(t *testing.T) {
	gatherer := newEmptyGatherer("", "")

	gatheringFunctions, err := gatherer.GetGatheringFunctions(context.Background())
	assert.NoError(t, err)

	assert.Len(t, gatheringFunctions, 2)
	_, found := gatheringFunctions["conditional_gatherer_rules"]
	assert.True(t, found)
}

func Test_Gatherer_GetGatheringFunctions_ConditionIsSatisfied(t *testing.T) {
	gatherer := newEmptyGatherer("", "")

	ctx := context.Background()
	err := gatherer.updateAlertsCache(ctx, newFakeClientWithAlerts("SamplesImagestreamImportFailing"))
	assert.NoError(t, err)

	gatheringFunctions, err := gatherer.GetGatheringFunctions(ctx)
	assert.NoError(t, err)

	assert.Len(t, gatheringFunctions, 4)

	_, found := gatheringFunctions["conditional_gatherer_rules"]
	assert.True(t, found)

	_, found = gatheringFunctions["logs_of_namespace/namespace=openshift-cluster-samples-operator,tail_lines=100"]
	assert.True(t, found)

	_, found = gatheringFunctions["image_streams_of_namespace/namespace=openshift-cluster-samples-operator"]
	assert.True(t, found)

	firing, err := gatherer.isAlertFiring("SamplesImagestreamImportFailing")
	assert.NoError(t, err)
	assert.True(t, firing)

	err = gatherer.updateAlertsCache(ctx, newFakeClientWithAlerts("OtherAlert"))
	assert.NoError(t, err)

	gatheringFunctions, err = gatherer.GetGatheringFunctions(ctx)
	assert.NoError(t, err)

	assert.Len(t, gatheringFunctions, 2)

	_, found = gatheringFunctions["conditional_gatherer_rules"]
	assert.True(t, found)

	_, found = gatheringFunctions["logs_of_namespace/namespace=openshift-cluster-samples-operator,tail_lines=100"]
	assert.False(t, found)

	_, found = gatheringFunctions["image_streams_of_namespace/namespace=openshift-cluster-samples-operator"]
	assert.False(t, found)

	firing, err = gatherer.isAlertFiring("SamplesImagestreamImportFailing")
	assert.NoError(t, err)
	assert.False(t, firing)
}

func Test_getConditionalGatheringFunctionName(t *testing.T) {
	res, err := getConditionalGatheringFunctionName("func", map[string]interface{}{
		"param1": "test",
		"param2": 5,
		"param3": "9",
		"param4": "",
	})
	assert.NoError(t, err)
	assert.Equal(t, "func/param1=test,param2=5,param3=9", res)
}

func newFakeClientWithAlerts(alerts ...string) *fake.RESTClient {
	var results []string
	for _, alert := range alerts {
		results = append(results, fmt.Sprintf(`{
			"metric": {
				"__name__": "ALERTS",
				"alertname": "%v",
				"alertstate": "firing",
				"severity": "critical"
			},
			"value": [1.0, "1"]
		}`, alert))
	}
	response := fmt.Sprintf(`{
		"status": "success",
		"data": {
			"resultType": "vector",
			"result": [
				%v
			]
		}
	}`, strings.Join(results, ","))

	fakeClient := &fake.RESTClient{
		NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		Client: fake.CreateHTTPClient(func(request *http.Request) (*http.Response, error) {
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(response + "\n")),
			}
			return resp, nil
		}),
	}
	return fakeClient
}

func Test_Gatherer_doesClusterVersionMatch(t *testing.T) {
	gatherer := newEmptyGatherer("", "")

	type testCase struct {
		expectedVersion string
		shouldMatch     bool
	}

	gatherer.clusterVersion = "4.8.0-0.nightly-2021-06-13-101614"

	for _, testCase := range []testCase{
		{
			expectedVersion: "4",
			shouldMatch:     false,
		},
		{
			expectedVersion: "4.8",
			shouldMatch:     false,
		},
		{
			expectedVersion: "4.8.0",
			shouldMatch:     true,
		},
		{
			expectedVersion: "4.8.x",
			shouldMatch:     true,
		},
		{
			expectedVersion: "4.8.1",
			shouldMatch:     false,
		},
		{
			expectedVersion: ">=4.8.0",
			shouldMatch:     true,
		},
		{
			expectedVersion: ">1.0.0 <2.0.0",
			shouldMatch:     false,
		},
		{
			expectedVersion: ">1.0.0 <2.0.0 || >=3.0.0",
			shouldMatch:     true,
		},
		{
			expectedVersion: "4.8.0-0.nightly-2021-06-13-101614",
			shouldMatch:     true,
		},
		{
			expectedVersion: "4.8.0-0.ci-2021-06-13-101614",
			shouldMatch:     false,
		},
		{
			expectedVersion: "4.8.0-1.nightly-2021-06-13-101614",
			shouldMatch:     false,
		},
	} {
		doesMatch, err := gatherer.doesClusterVersionMatch(testCase.expectedVersion)
		if err != nil {
			assert.Error(t, err)
		}

		assert.Equalf(t, testCase.shouldMatch, doesMatch, "test case is '%v'", testCase)
	}
}

func TestGetGatheringFunctions(t *testing.T) {
	tests := []struct {
		name                  string
		endpoint              string
		remoteConfig          string
		expectedErrMsg        string
		expectRemoteConfigErr bool
	}{
		{
			name:                  "remote configuration is available and can be parsed",
			endpoint:              "/gathering_rules",
			remoteConfig:          "",
			expectedErrMsg:        "",
			expectRemoteConfigErr: false,
		},
		{
			name:                  "remote configuration is not available",
			endpoint:              "not valid endpoint",
			remoteConfig:          "",
			expectedErrMsg:        "endpoint not supported",
			expectRemoteConfigErr: true,
		},
		{
			name:                  "remote configuration is available, but cannot be parsed",
			endpoint:              "/gathering_rules",
			remoteConfig:          `{not json}`,
			expectedErrMsg:        "invalid character 'n' looking for beginning of object key string",
			expectRemoteConfigErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gatherer := newEmptyGatherer(tt.remoteConfig, tt.endpoint)
			_, err := gatherer.GetGatheringFunctions(context.Background())
			if err != nil {
				assert.EqualError(t, err, tt.expectedErrMsg)
			}
			assert.Equal(t, tt.expectRemoteConfigErr, errors.As(err, &RemoteConfigError{}))
		})
	}
}

func newEmptyGatherer(remoteConfig string, conditionalGathererEndpoint string) *Gatherer { // nolint:gocritic
	if len(remoteConfig) == 0 {
		remoteConfig = `{
			"version": "1.0.0",
			"conditional_gathering_rules": [{
				"conditions": [
					{
						"type": "` + string(AlertIsFiring) + `",
						"alert": { "name": "SamplesImagestreamImportFailing" }
					}
				],
				"gathering_functions": {
					"logs_of_namespace": {
						"namespace": "openshift-cluster-samples-operator",
						"tail_lines": 100
					},
					"image_streams_of_namespace": {
						"namespace": "openshift-cluster-samples-operator"
					}
				}
			}],
			"container_logs":[]
		}`
	}
	if conditionalGathererEndpoint == "" {
		conditionalGathererEndpoint = "/gathering_rules"
	}
	testConf := &config.InsightsConfiguration{
		DataReporting: config.DataReporting{
			ConditionalGathererEndpoint: conditionalGathererEndpoint,
		},
	}
	mockConfigurator := config.NewMockConfigMapConfigurator(testConf)

	return New(
		nil,
		nil,
		nil,
		mockConfigurator,
		&MockGatheringRulesServiceClient{Conf: remoteConfig},
	)
}

type MockGatheringRulesServiceClient struct {
	Conf string
	err  error
}

func (s *MockGatheringRulesServiceClient) GetWithPathParam(_ context.Context, endpoint, _ string, _ bool) (*http.Response, error) {
	if s.err != nil {
		return nil, s.err
	}
	if strings.HasSuffix(endpoint, "gathering_rules") {
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(s.Conf)),
		}
		return resp, nil
	}

	return nil, fmt.Errorf("endpoint not supported")
}
