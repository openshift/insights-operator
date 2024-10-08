package conditional

import (
	"context"
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

var testRemoteConfig = `{
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

func Test_Gatherer_Basic(t *testing.T) {
	t.Setenv("RELEASE_VERSION", "1.2.3")
	gatherer := newEmptyGatherer("", "")

	assert.Equal(t, "conditional", gatherer.GetName())
	gatheringFunctions, err := gatherer.GetGatheringFunctions(context.Background())
	assert.NoError(t, err)
	assert.Len(t, gatheringFunctions, 3)

	assert.Implements(t, (*gatherers.Interface)(nil), gatherer)
	var g interface{} = gatherer
	_, ok := g.(gatherers.CustomPeriodGatherer)
	assert.False(t, ok, "should NOT implement gather.CustomPeriodGatherer")
}

func Test_Gatherer_GetGatheringFunctions(t *testing.T) {
	t.Setenv("RELEASE_VERSION", "1.2.3")
	gatherer := newEmptyGatherer("", "")

	ctx := context.Background()
	err := gatherer.updateAlertsCache(ctx, newFakeClientWithAlerts("SamplesImagestreamImportFailing"))
	assert.NoError(t, err)

	gatheringFunctions, err := gatherer.GetGatheringFunctions(ctx)
	assert.NoError(t, err)
	assert.Len(t, gatheringFunctions, 5)
	_, found := gatheringFunctions["conditional_gatherer_rules"]
	assert.True(t, found)
}

func Test_Gatherer_GetGatheringFunctions_BuiltInConfigIsUsed(t *testing.T) {
	t.Setenv("RELEASE_VERSION", "1.2.3")
	gatherer := newEmptyGatherer("", "")

	ctx := context.Background()
	err := gatherer.updateAlertsCache(ctx, newFakeClientWithAlerts("SamplesImagestreamImportFailing"))
	assert.NoError(t, err)

	gatheringFunctions, err := gatherer.GetGatheringFunctions(ctx)
	assert.NoError(t, err)
	assert.Len(t, gatheringFunctions, 5)
	_, found := gatheringFunctions["conditional_gatherer_rules"]
	assert.True(t, found)

	// the service suddenly died

	gatherer.insightsCli = &MockGatheringRulesServiceClient{
		err: fmt.Errorf("404"),
	}

	gatheringFunctions, err = gatherer.GetGatheringFunctions(ctx)
	// no error because built-in config should be used
	assert.NoError(t, err)
	assert.Equal(t, gatherers.RemoteConfigStatus{
		Err:             fmt.Errorf("404"),
		AvailableReason: "NotAvailable",
		ValidReason:     "NoValidation",
		ConfigData:      []byte(defaultRemoteConfiguration),
		ConfigAvailable: false,
		ConfigValid:     true,
	}, gatherer.remoteConfigStatus)
	assert.Len(t, gatheringFunctions, 5)
	_, found = gatheringFunctions["conditional_gatherer_rules"]
	assert.True(t, found)
}

func Test_Gatherer_GetGatheringFunctions_NoConditionsAreSatisfied(t *testing.T) {
	t.Setenv("RELEASE_VERSION", "1.2.3")
	gatherer := newEmptyGatherer("", "")

	gatheringFunctions, err := gatherer.GetGatheringFunctions(context.Background())
	assert.NoError(t, err)

	assert.Len(t, gatheringFunctions, 3)
	_, found := gatheringFunctions["conditional_gatherer_rules"]
	assert.True(t, found)
}

func Test_Gatherer_GetGatheringFunctions_ConditionIsSatisfied(t *testing.T) {
	t.Setenv("RELEASE_VERSION", "1.2.3")
	gatherer := newEmptyGatherer("", "")

	ctx := context.Background()
	err := gatherer.updateAlertsCache(ctx, newFakeClientWithAlerts("SamplesImagestreamImportFailing"))
	assert.NoError(t, err)

	gatheringFunctions, err := gatherer.GetGatheringFunctions(ctx)
	assert.NoError(t, err)

	assert.Len(t, gatheringFunctions, 5)

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

	assert.Len(t, gatheringFunctions, 3)

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
		name                 string
		endpoint             string
		remoteConfig         string
		releaseVersionEnvVar string
		remoteConfigStatus   gatherers.RemoteConfigStatus
	}{
		{
			name:                 "remote configuration is available and can be parsed",
			endpoint:             "/gathering_rules",
			releaseVersionEnvVar: "1.2.3",
			remoteConfig:         "",
			remoteConfigStatus: gatherers.RemoteConfigStatus{
				ConfigAvailable: true,
				ConfigValid:     true,
				AvailableReason: AsExpectedReason,
				ValidReason:     AsExpectedReason,
				Err:             nil,
				ConfigData:      []byte(testRemoteConfig),
			},
		},
		{
			name:                 "remote configuration is not available",
			endpoint:             "not valid endpoint",
			releaseVersionEnvVar: "1.2.3",
			remoteConfig:         "",
			remoteConfigStatus: gatherers.RemoteConfigStatus{
				ConfigAvailable: false,
				ConfigValid:     false,
				AvailableReason: NotAvailableReason,
				ValidReason:     "NoValidation",
				ConfigData:      []byte(defaultRemoteConfiguration),
				Err:             fmt.Errorf("endpoint not supported"),
			},
		},
		{
			name:                 "remote configuration is available, but cannot be parsed",
			endpoint:             "/gathering_rules",
			remoteConfig:         `{not json}`,
			releaseVersionEnvVar: "1.2.3",
			remoteConfigStatus: gatherers.RemoteConfigStatus{
				ConfigAvailable: true,
				ConfigValid:     false,
				AvailableReason: AsExpectedReason,
				ValidReason:     InvalidReason,
				ConfigData:      []byte(defaultRemoteConfiguration),
				Err:             fmt.Errorf("invalid character 'n' looking for beginning of object key string"),
			},
		},
		{
			name:                 "remote configuration is not available, because RELEASE_VERSION is set with empty",
			endpoint:             "/gathering_rules",
			releaseVersionEnvVar: "",
			remoteConfig:         "",
			remoteConfigStatus: gatherers.RemoteConfigStatus{
				ConfigAvailable: false,
				ConfigValid:     false,
				AvailableReason: NotAvailableReason,
				ValidReason:     "NoValidation",
				ConfigData:      []byte(defaultRemoteConfiguration),
				Err:             fmt.Errorf("environmental variable RELEASE_VERSION is not set or has empty value"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("RELEASE_VERSION", tt.releaseVersionEnvVar)
			gatherer := newEmptyGatherer(tt.remoteConfig, tt.endpoint)
			_, err := gatherer.GetGatheringFunctions(context.Background())
			assert.NoError(t, err)
			assert.Equal(t, tt.remoteConfigStatus.ConfigAvailable, gatherer.RemoteConfigStatus().ConfigAvailable)
			assert.Equal(t, tt.remoteConfigStatus.ConfigValid, gatherer.RemoteConfigStatus().ConfigValid)
			assert.Equal(t, tt.remoteConfigStatus.ConfigData, gatherer.RemoteConfigStatus().ConfigData)
			assert.Equal(t, tt.remoteConfigStatus.AvailableReason, gatherer.RemoteConfigStatus().AvailableReason)
			assert.Equal(t, tt.remoteConfigStatus.ValidReason, gatherer.RemoteConfigStatus().ValidReason)
			if tt.remoteConfigStatus.Err != nil {
				assert.EqualError(t, gatherer.remoteConfigStatus.Err, tt.remoteConfigStatus.Err.Error())
			}
		})
	}
}

func TestBuiltInConfigIsUsed(t *testing.T) {
	// simulate that the remote config is not available
	gatherer := newEmptyGatherer("", "non existing endpoint")
	// override default configuration
	defaultRemoteConfiguration = `{
	"container_logs": [
		{"namespace": "openshift-test-ns","pod_name_regex": "test-name","messages":["test"]}
		],
	"conditional_gathering_rules":[
		{
			"conditions": [
                {
                    "alert": {
                        "name": "APIRemovedInNextEUSReleaseInUse"
                    },
                    "type": "alert_is_firing"
                }
            ],
            "gathering_functions": {
                "api_request_counts_of_resource_from_alert": {
                    "alert_name": "APIRemovedInNextEUSReleaseInUse"
                }
            }
		}
	]
	}`
	gatheringClosures, err := gatherer.GetGatheringFunctions(context.Background())
	assert.NoError(t, err)
	containerLogClosure, ok := gatheringClosures["rapid_container_logs"]
	assert.True(t, ok)
	assert.NotNil(t, containerLogClosure)
	assert.False(t, gatherer.RemoteConfigStatus().ConfigAvailable)
	assert.Equal(t, defaultRemoteConfiguration, string(gatherer.RemoteConfigStatus().ConfigData))
}

func newEmptyGatherer(remoteConfig string, conditionalGathererEndpoint string) *Gatherer { // nolint:gocritic
	if len(remoteConfig) == 0 {
		remoteConfig = testRemoteConfig
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
