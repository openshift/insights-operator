package conditional

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest/fake"

	"github.com/openshift/insights-operator/pkg/gatherers"
)

func newEmptyGatherer() *Gatherer {
	return &Gatherer{}
}

func Test_Gatherer_Basic(t *testing.T) {
	gatherer := newEmptyGatherer()
	assert.Equal(t, "conditional", gatherer.GetName())
	gatheringFunctions, err := gatherer.GetGatheringFunctions(context.TODO())
	assert.NoError(t, err)
	assert.Greater(t, len(gatheringFunctions), 0)

	assert.Implements(t, (*gatherers.Interface)(nil), gatherer)

	var g interface{} = gatherer
	_, ok := g.(gatherers.CustomPeriodGatherer)
	assert.False(t, ok, "should NOT implement gather.CustomPeriodGatherer")
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
		`ALERTS{alertname="SamplesImagestreamImportFailing",alertstate="firing"} 1 1621618110163`,
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
		`ALERTS{alertname="OtherAlert",alertstate="firing"} 1 1621618110163`,
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
	res := getConditionalGatheringFunctionName("func", map[string]interface{}{
		"param1": "test",
		"param2": 5,
		"param3": "9",
		"param4": "",
	})
	assert.Equal(t, "func/param1=test,param2=5,param3=9", res)
}

func Test_getInterfaceFromMap(t *testing.T) {
	i, err := getInterfaceFromMap(map[string]interface{}{}, "key")
	assert.Nil(t, i)
	assert.EqualError(t, err, "unable to find a value with key 'key' in the map 'map[]'")

	val, err := getInterfaceFromMap(map[string]interface{}{"key": "val"}, "key")
	assert.NoError(t, err)
	assert.Equal(t, "val", val)
}

func Test_getStringFromMap(t *testing.T) {
	val, err := getStringFromMap(map[string]interface{}{}, "key")
	assert.Empty(t, val)
	assert.EqualError(t, err, "unable to find a value with key 'key' in the map 'map[]'")

	val, err = getStringFromMap(map[string]interface{}{"key": 9}, "key")
	assert.Empty(t, val)
	assert.EqualError(t, err, "unable to convert '9' to string")

	val, err = getStringFromMap(map[string]interface{}{"key": "val"}, "key")
	assert.NoError(t, err)
	assert.Equal(t, "val", val)
}

func Test_getInt64FromMap(t *testing.T) {
	val, err := getInt64FromMap(map[string]interface{}{}, "key")
	assert.Empty(t, val)
	assert.EqualError(t, err, "unable to find a value with key 'key' in the map 'map[]'")

	val, err = getInt64FromMap(map[string]interface{}{"key": "val"}, "key")
	assert.Empty(t, val)
	assert.EqualError(t, err, `strconv.ParseInt: parsing "val": invalid syntax`)

	val, err = getInt64FromMap(map[string]interface{}{"key": 9}, "key")
	assert.NoError(t, err)
	assert.Equal(t, int64(9), val)

	val, err = getInt64FromMap(map[string]interface{}{"key": "6"}, "key")
	assert.NoError(t, err)
	assert.Equal(t, int64(6), val)
}

func Test_getPositiveInt64FromMap(t *testing.T) {
	val, err := getPositiveInt64FromMap(map[string]interface{}{}, "key")
	assert.Empty(t, val)
	assert.EqualError(t, err, "unable to find a value with key 'key' in the map 'map[]'")

	val, err = getPositiveInt64FromMap(map[string]interface{}{"key": "-6"}, "key")
	assert.Empty(t, val)
	assert.EqualError(t, err, "positive int expected, got '-6'")

	val, err = getPositiveInt64FromMap(map[string]interface{}{"key": "6"}, "key")
	assert.NoError(t, err)
	assert.Equal(t, int64(6), val)
}

func Test_Gatherer_GatherConditionalGathererRules(t *testing.T) {
	gatherer := newEmptyGatherer()
	records, errs := gatherer.GatherConditionalGathererRules(context.TODO())
	assert.Empty(t, errs)

	assert.Len(t, records, 1)
	assert.Equal(t, "insights-operator/conditional-gatherer-rules", records[0].Name)

	item, err := records[0].Item.Marshal(context.TODO())
	assert.NoError(t, err)

	var gotGatheringRules []gatheringRule
	err = json.Unmarshal(item, &gotGatheringRules)
	assert.NoError(t, err)

	assert.Len(t, gotGatheringRules, 1)
}

func newFakeClientWithMetrics(metrics string) *fake.RESTClient {
	fakeClient := &fake.RESTClient{
		NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
		Client: fake.CreateHTTPClient(func(request *http.Request) (*http.Response, error) {
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Body:       ioutil.NopCloser(strings.NewReader(metrics)),
			}
			return resp, nil
		}),
	}
	return fakeClient
}
