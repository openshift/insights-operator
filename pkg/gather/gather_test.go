package gather

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openshift/insights-operator/pkg/anonymization"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/recorder"
)

func Test_GetListOfEnabledFunctionForGatherer(t *testing.T) {
	list := []string{
		"clusterconfig/container_images",
		"clusterconfig/nodes",
		"clusterconfig/authentication",
		"othergatherer/some_function",
	}

	all, functions := getListOfEnabledFunctionForGatherer("clusterconfig", list)
	assert.False(t, all)
	assert.ElementsMatch(t, functions, []string{
		"container_images",
		"nodes",
		"authentication",
	})

	all, functions = getListOfEnabledFunctionForGatherer("othergatherer", list)
	assert.False(t, all)
	assert.ElementsMatch(t, functions, []string{
		"some_function",
	})

	all, functions = getListOfEnabledFunctionForGatherer("NotExistingGatherer", list)
	assert.False(t, all)
	assert.Empty(t, functions)

	all, functions = getListOfEnabledFunctionForGatherer("", list)
	assert.Empty(t, functions)

	list = []string{
		"clusterconfig/container_images",
		AllGatherersConst,
		"clusterconfig/authentication",
		"othergatherer/some_function",
	}

	all, functions = getListOfEnabledFunctionForGatherer("clusterconfig", list)
	assert.True(t, all)
	assert.Empty(t, functions)

	all, functions = getListOfEnabledFunctionForGatherer("othergatherer", list)
	assert.True(t, all)
	assert.Empty(t, functions)

	all, functions = getListOfEnabledFunctionForGatherer("", list)
	assert.True(t, all)
	assert.Empty(t, functions)

	list = []string{}

	all, functions = getListOfEnabledFunctionForGatherer("clusterconfig", list)
	assert.False(t, all)
	assert.Empty(t, functions)
}

func Test_sumErrors(t *testing.T) {
	err := sumErrors([]error{})
	assert.NoError(t, err)

	err = sumErrors([]error{
		fmt.Errorf("test error"),
	})
	assert.EqualError(t, err, "test error")

	err = sumErrors([]error{
		fmt.Errorf("error 1"),
		fmt.Errorf("error 2"),
		fmt.Errorf("error 3"),
	})
	assert.EqualError(t, err, "error 1, error 2, error 3")

	err = sumErrors([]error{
		fmt.Errorf("error 3"),
		fmt.Errorf("error 3"),
		fmt.Errorf("error 2"),
		fmt.Errorf("error 1"),
		fmt.Errorf("error 5"),
	})
	assert.EqualError(t, err, "error 1, error 2, error 3, error 5")
}

func Test_StartGatheringConcurrently(t *testing.T) {
	gatherer := &MockGatherer{SomeField: "some_value"}

	resultsChan, err := startGatheringConcurrently(context.Background(), gatherer, []string{AllGatherersConst})
	assert.NoError(t, err)

	results := gatherResultsFromChannel(resultsChan)
	assert.Len(t, results, 5)

	for i := range results {
		results[i].TimeElapsed = 0
	}

	assert.ElementsMatch(t, results, []GatheringFunctionResult{
		{
			FunctionName: "name",
			Records: []record.Record{
				{
					Name: "name",
					Item: record.JSONMarshaller{Object: "mock_gatherer"},
				},
			},
		},
		{
			FunctionName: "some_field",
			Records: []record.Record{
				{
					Name: "some_field",
					Item: record.JSONMarshaller{Object: "some_value"},
				},
			},
		},
		{
			FunctionName: "3_records",
			Records: []record.Record{
				{
					Name: "record_1",
					Item: record.JSONMarshaller{Object: "data 1"},
				},
				{
					Name: "record_2",
					Item: record.JSONMarshaller{Object: "data 2"},
				},
				{
					Name: "record_3",
					Item: record.JSONMarshaller{Object: "data 3"},
				},
			},
		},
		{
			FunctionName: "errors",
			Errs: []error{
				fmt.Errorf("error1"),
				fmt.Errorf("error2"),
				fmt.Errorf("error3"),
			},
			IgnoreErrors: false,
		},
		{
			FunctionName: "panic",
			Panic:        "test panic",
			IgnoreErrors: false,
		},
	})

	resultsChan, err = startGatheringConcurrently(context.Background(), gatherer, []string{
		"mock_gatherer/some_field",
	})
	assert.NoError(t, err)

	results = gatherResultsFromChannel(resultsChan)
	assert.Len(t, results, 1)
	results[0].TimeElapsed = 0

	assert.ElementsMatch(t, results, []GatheringFunctionResult{
		{
			FunctionName: "some_field",
			Records: []record.Record{
				{
					Name: "some_field",
					Item: record.JSONMarshaller{Object: "some_value"},
				},
			},
		},
	})

	resultsChan, err = startGatheringConcurrently(context.Background(), gatherer, []string{
		"mock_gatherer/name",
		"mock_gatherer/3_records",
	})

	results = gatherResultsFromChannel(resultsChan)
	assert.Len(t, results, 2)
	for i := range results {
		results[i].TimeElapsed = 0
	}

	assert.ElementsMatch(t, results, []GatheringFunctionResult{
		{
			FunctionName: "name",
			Records: []record.Record{
				{
					Name: "name",
					Item: record.JSONMarshaller{Object: "mock_gatherer"},
				},
			},
		},
		{
			FunctionName: "3_records",
			Records: []record.Record{
				{
					Name: "record_1",
					Item: record.JSONMarshaller{Object: "data 1"},
				},
				{
					Name: "record_2",
					Item: record.JSONMarshaller{Object: "data 2"},
				},
				{
					Name: "record_3",
					Item: record.JSONMarshaller{Object: "data 3"},
				},
			},
		},
	})
}

func Test_StartGatheringConcurrently_Error(t *testing.T) {
	gatherer := &MockGatherer{SomeField: "some_value"}

	resultsChan, err := startGatheringConcurrently(context.Background(), gatherer, []string{})
	assert.EqualError(t, err, "no gather functions are specified to run")
	assert.Nil(t, resultsChan)

	resultsChan, err = startGatheringConcurrently(context.Background(), gatherer, []string{""})
	assert.EqualError(t, err, "no gather functions are specified to run")
	assert.Nil(t, resultsChan)

	resultsChan, err = startGatheringConcurrently(context.Background(), gatherer, []string{"not existing function"})
	assert.EqualError(t, err, "no gather functions are specified to run")
	assert.Nil(t, resultsChan)
}

func Test_CollectAndRecord(t *testing.T) {
	gatherer := &MockGatherer{
		SomeField: "some_value",
		CanFail:   true,
	}
	mockRecorder := &recorder.MockRecorder{}
	mockConfigurator := &config.MockConfigurator{Conf: &config.Controller{
		Gather:                  []string{AllGatherersConst},
		EnableGlobalObfuscation: true,
	}}
	anonymizer, err := anonymization.NewAnonymizer("", nil)
	assert.NoError(t, err)

	functionReports, err := CollectAndRecordGatherer(context.Background(), gatherer, mockRecorder, mockConfigurator)
	assert.NoError(t, err)

	err = RecordArchiveMetadata(functionReports, mockRecorder, anonymizer)
	assert.NoError(t, err)

	assert.Len(t, mockRecorder.Records, 6)
	assertMetadataOneGatherer(t, mockRecorder.Records, true, []GathererFunctionReport{
		{
			FuncName:     "mock_gatherer/name",
			RecordsCount: 1,
		},
		{
			FuncName:     "mock_gatherer/some_field",
			RecordsCount: 1,
		},
		{
			FuncName:     "mock_gatherer/3_records",
			RecordsCount: 3,
		},
		{
			FuncName: "mock_gatherer/errors",
			Errors: []string{
				"error1",
				"error2",
				"error3",
			},
		},
		{
			FuncName: "mock_gatherer/panic",
			Errors:   []string{"test panic"},
			Panic:    "test panic",
		},
	})
	assertRecordsOneGatherer(t, mockRecorder.Records, []record.Record{
		{
			Name: "name",
			Item: record.JSONMarshaller{Object: "mock_gatherer"},
		},
		{
			Name: "some_field",
			Item: record.JSONMarshaller{Object: "some_value"},
		},
		{
			Name: "record_1",
			Item: record.JSONMarshaller{Object: "data 1"},
		},
		{
			Name: "record_2",
			Item: record.JSONMarshaller{Object: "data 2"},
		},
		{
			Name: "record_3",
			Item: record.JSONMarshaller{Object: "data 3"},
		},
	})
}

func Test_CollectAndRecord_Error(t *testing.T) {
	gatherer := &MockGatherer{
		CanFail: false,
	}
	mockRecorder := &recorder.MockRecorder{}
	mockConfigurator := &config.MockConfigurator{Conf: &config.Controller{
		Gather:                  []string{"mock_gatherer/errors"},
		EnableGlobalObfuscation: false,
	}}

	functionReports, err := CollectAndRecordGatherer(context.Background(), gatherer, mockRecorder, mockConfigurator)
	assert.EqualError(
		t,
		err,
		"gatherer mock_gatherer's function errors failed with error: error1, "+
			"gatherer mock_gatherer's function errors failed with error: error2, "+
			"gatherer mock_gatherer's function errors failed with error: error3",
	)

	err = RecordArchiveMetadata(functionReports, mockRecorder, nil)
	assert.NoError(t, err)

	assert.Len(t, mockRecorder.Records, 1)
	assertMetadataOneGatherer(t, mockRecorder.Records, false, []GathererFunctionReport{
		{
			FuncName:     "mock_gatherer/errors",
			RecordsCount: 0,
			Errors: []string{
				"error1",
				"error2",
				"error3",
			},
		},
	})
}

func Test_CollectAndRecord_Panic(t *testing.T) {
	gatherer := &MockGatherer{
		CanFail: false,
	}
	mockRecorder := &recorder.MockRecorder{}
	mockConfigurator := &config.MockConfigurator{Conf: &config.Controller{
		Gather:                  []string{"mock_gatherer/panic"},
		EnableGlobalObfuscation: false,
	}}

	functionReports, err := CollectAndRecordGatherer(context.Background(), gatherer, mockRecorder, mockConfigurator)
	assert.EqualError(t, err, "gatherer mock_gatherer's function panic failed with error: test panic")
	assert.Len(t, functionReports, 1)
	functionReports[0].Duration = 0
	assert.ElementsMatch(t, functionReports, []GathererFunctionReport{{
		FuncName: "mock_gatherer/panic",
		Errors:   []string{"test panic"},
		Panic:    "test panic",
	}})
	assert.Len(t, mockRecorder.Records, 0)
}

func assertMetadataOneGatherer(
	t testing.TB,
	records []record.Record,
	isGlobalObfuscationEnabled bool,
	statusReports []GathererFunctionReport,
) {
	assert.GreaterOrEqual(t, len(records), 1)

	var metadataBytes []byte

	for _, rec := range records {
		if strings.HasSuffix(rec.Name, "insights-operator/gathers") {
			bytes, err := rec.Item.Marshal(context.Background())
			assert.NoError(t, err)

			metadataBytes = bytes
		}
	}

	var archiveMetadata ArchiveMetadata

	err := json.Unmarshal(metadataBytes, &archiveMetadata)
	assert.NoError(t, err)

	for i := range archiveMetadata.StatusReports {
		archiveMetadata.StatusReports[i].Duration = 0
	}

	assert.Equal(t, isGlobalObfuscationEnabled, archiveMetadata.IsGlobalObfuscationEnabled)
	assert.ElementsMatch(t, statusReports, archiveMetadata.StatusReports)
}

func assertRecordsOneGatherer(t testing.TB, records []record.Record, expectedRecords []record.Record) {
	var recordsWithoutMetadata []record.Record
	for _, r := range records {
		if !strings.HasSuffix(r.Name, "insights-operator/gathers") {
			recordsWithoutMetadata = append(recordsWithoutMetadata, r)
		}
	}

	assert.ElementsMatch(t, recordsWithoutMetadata, expectedRecords)
}

func gatherResultsFromChannel(resultsChan chan GatheringFunctionResult) []GatheringFunctionResult {
	var results []GatheringFunctionResult
	for result := range resultsChan {
		results = append(results, result)
	}

	return results
}
