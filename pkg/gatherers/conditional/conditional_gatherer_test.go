package conditional

import (
	"context"
	"encoding/json"
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
	return New(nil, nil)
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
	err := gatherer.updateAlertsCacheFromClient(context.TODO(), newFakeClientWithMetrics(
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
					Params: AlertIsFiringConditionParams{
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

	err := gatherer.updateAlertsCacheFromClient(context.TODO(), newFakeClientWithMetrics(
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

	err := gatherer.updateAlertsCacheFromClient(context.TODO(), newFakeClientWithMetrics(
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

	assert.True(t, gatherer.isAlertFiring("SamplesImagestreamImportFailing"))

	err = gatherer.updateAlertsCacheFromClient(context.TODO(), newFakeClientWithMetrics(
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

	assert.False(t, gatherer.isAlertFiring("SamplesImagestreamImportFailing"))
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

func Test_Gatherer_GatherConditionalGathererRules(t *testing.T) {
	gatherer := newEmptyGatherer()
	records, errs := gatherer.GatherConditionalGathererRules(context.TODO())
	assert.Empty(t, errs)

	assert.Len(t, records, 1)
	assert.Equal(t, "insights-operator/conditional-gatherer-rules", records[0].Name)

	item, err := records[0].Item.Marshal(context.TODO())
	assert.NoError(t, err)

	var gotGatheringRules []GatheringRule
	err = json.Unmarshal(item, &gotGatheringRules)
	assert.NoError(t, err)

	assert.Len(t, gotGatheringRules, 4)
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
