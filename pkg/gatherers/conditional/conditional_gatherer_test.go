package conditional

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest/fake"

	"github.com/openshift/insights-operator/pkg/gatherers"
)

func newEmptyGatherer() *Gatherer {
	return New(nil, nil, nil)
}

func Test_Gatherer_Basic(t *testing.T) {
	gatherer := newEmptyGatherer()
	assert.Equal(t, "conditional", gatherer.GetName())
	gatheringFunctions, err := gatherer.GetGatheringFunctions(context.TODO())
	assert.NoError(t, err)
	assert.Len(t, gatheringFunctions, 1)

	assert.Implements(t, (*gatherers.Interface)(nil), gatherer)

	var g interface{} = gatherer
	_, ok := g.(gatherers.CustomPeriodGatherer)
	assert.False(t, ok, "should NOT implement gather.CustomPeriodGatherer")
}

func Test_Gatherer_GetGatheringFunctions(t *testing.T) {
	gatherer := newEmptyGatherer()
	err := gatherer.updateAlertsCache(context.TODO(), newFakeClientWithMetrics(
		`ALERTS{alertname="SamplesImagestreamImportFailing",alertstate="firing"} 1 1621618110163`,
	))
	assert.NoError(t, err)

	gatheringFunctions, err := gatherer.GetGatheringFunctions(context.TODO())
	assert.NoError(t, err)
	assert.Len(t, gatheringFunctions, 3)
	_, found := gatheringFunctions["conditional_gatherer_rules"]
	assert.True(t, found)
}

func Test_Gatherer_GetGatheringFunctions_InvalidConfig(t *testing.T) {
	gatherer := newEmptyGatherer()
	gatherer.gatheringRules = []GatheringRule{
		{
			Conditions: []ConditionWithParams{
				{
					Type: AlertIsFiring,
					Alert: &AlertConditionParams{
						Name: "SamplesImagestreamImportFailing",
					},
				},
			},
			GatheringFunctions: GatheringFunctions{
				GatherLogsOfNamespace: GatherLogsOfNamespaceParams{
					Namespace: "not-openshift-cluster-samples-operator",
					TailLines: 100,
				},
			},
		},
	} // invalid namespace (doesn't start with openshift-)

	err := gatherer.updateAlertsCache(context.TODO(), newFakeClientWithMetrics(
		`ALERTS{alertname="SamplesImagestreamImportFailing",alertstate="firing"} 1 1621618110163`,
	))
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
	gatherer := newEmptyGatherer()

	gatheringFunctions, err := gatherer.GetGatheringFunctions(context.TODO())
	assert.NoError(t, err)

	assert.Len(t, gatheringFunctions, 1)
	_, found := gatheringFunctions["conditional_gatherer_rules"]
	assert.True(t, found)
}

func Test_Gatherer_GetGatheringFunctions_ConditionIsSatisfied(t *testing.T) {
	gatherer := newEmptyGatherer()

	err := gatherer.updateAlertsCache(context.TODO(), newFakeClientWithMetrics(
		"ALERTS{alertname=\"SamplesImagestreamImportFailing\",alertstate=\"firing\"} 1 1621618110163\n",
	))
	assert.NoError(t, err)

	gatheringFunctions, err := gatherer.GetGatheringFunctions(context.TODO())
	assert.NoError(t, err)

	assert.Len(t, gatheringFunctions, 3)

	_, found := gatheringFunctions["conditional_gatherer_rules"]
	assert.True(t, found)

	_, found = gatheringFunctions["logs_of_namespace/namespace=openshift-cluster-samples-operator,tail_lines=100"]
	assert.True(t, found)

	_, found = gatheringFunctions["image_streams_of_namespace/namespace=openshift-cluster-samples-operator"]
	assert.True(t, found)

	firing, err := gatherer.isAlertFiring("SamplesImagestreamImportFailing")
	assert.NoError(t, err)
	assert.True(t, firing)

	err = gatherer.updateAlertsCache(context.TODO(), newFakeClientWithMetrics(
		"ALERTS{alertname=\"OtherAlert\",alertstate=\"firing\"} 1 1621618110163\n",
	))
	assert.NoError(t, err)

	gatheringFunctions, err = gatherer.GetGatheringFunctions(context.TODO())
	assert.NoError(t, err)

	assert.Len(t, gatheringFunctions, 1)

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

func newFakeClientWithMetrics(metrics string) *fake.RESTClient {
	fakeClient := &fake.RESTClient{
		NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		Client: fake.CreateHTTPClient(func(request *http.Request) (*http.Response, error) {
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(metrics + "\n")),
			}
			return resp, nil
		}),
	}
	return fakeClient
}

func Test_Gatherer_doesClusterVersionMatch(t *testing.T) {
	g := newEmptyGatherer()

	type testCase struct {
		expectedVersion string
		shouldMatch     bool
	}

	g.clusterVersion = "4.8.0-0.nightly-2021-06-13-101614"

	for _, testCase := range []testCase{
		{
			expectedVersion: "4.8.x",
			shouldMatch:     true,
		},
		{
			expectedVersion: "4.8.0",
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
	} {
		doesMatch, err := g.doesClusterVersionMatch(testCase.expectedVersion)
		if err != nil {
			assert.Error(t, err)
		}

		assert.Equal(t, testCase.shouldMatch, doesMatch)
	}
}
