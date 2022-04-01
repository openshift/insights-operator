package gather

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/openshift/insights-operator/pkg/anonymization"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/gatherers"
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
	assert.False(t, all)
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

// nolint: funlen
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
		},
		{
			FunctionName: "panic",
			Panic:        "test panic",
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
	assert.NoError(t, err)
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

func Test_CollectAndRecordGatherer(t *testing.T) {
	gatherer := &MockGatherer{
		SomeField: "some_value",
	}
	mockRecorder := &recorder.MockRecorder{}
	mockConfigurator := config.NewMockConfigurator(&config.Controller{
		EnableGlobalObfuscation: true,
	})
	anonymizer, err := anonymization.NewAnonymizer("", nil, nil)
	assert.NoError(t, err)

	functionReports, err := CollectAndRecordGatherer(context.Background(), gatherer, mockRecorder, mockConfigurator)
	assert.Error(t, err)

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

func Test_CollectAndRecordGatherer_Error(t *testing.T) {
	gatherer := &MockGatherer{}
	mockRecorder := &recorder.MockRecorder{}
	mockConfigurator := config.NewMockConfigurator(&config.Controller{
		Gather:                  []string{"mock_gatherer/errors"},
		EnableGlobalObfuscation: false,
	})

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

func Test_CollectAndRecordGatherer_Panic(t *testing.T) {
	gatherer := &MockGatherer{}
	mockRecorder := &recorder.MockRecorder{}
	mockConfigurator := config.NewMockConfigurator(&config.Controller{
		Gather:                  []string{"mock_gatherer/panic"},
		EnableGlobalObfuscation: false,
	})

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

func Test_CollectAndRecordGatherer_DuplicateRecords(t *testing.T) {
	gatherer := &MockGathererWithProvidedFunctions{Functions: map[string]gatherers.GatheringClosure{
		"function_1": {Run: func(ctx context.Context) ([]record.Record, []error) {
			return []record.Record{{
				Name: "record_1",
				Item: record.JSONMarshaller{Object: "content_1"},
			}}, nil
		}},
		"function_2": {Run: func(ctx context.Context) ([]record.Record, []error) {
			return []record.Record{{
				Name: "record_1",
				Item: record.JSONMarshaller{Object: "content_2"},
			}}, nil
		}},
		"function_3": {Run: func(ctx context.Context) ([]record.Record, []error) {
			return []record.Record{{
				Name: "record_2",
				Item: record.JSONMarshaller{Object: "content_1"},
			}}, nil
		}},
	}}
	mockDriver := &MockDriver{}
	rec := recorder.New(mockDriver, time.Second, nil)
	mockConfigurator := config.NewMockConfigurator(nil)

	functionReports, err := CollectAndRecordGatherer(context.Background(), gatherer, rec, mockConfigurator)
	assert.EqualError(
		t, err,
		"unable to record gatherer mock_gatherer_with_provided_functions function function_2' result "+
			"record_1 because of the error: A record with the name record_1.json was already recorded and had "+
			"fingerprint 4dc9ed5d5654c1c2b6da4629ac8a0b62 , overwriting with data having "+
			"fingerprint b0560bacc0b4956efd5b527f9a27910e",
	)
	assert.NotEmpty(t, functionReports)
	assert.Len(t, functionReports, 3)

	sort.Slice(functionReports, func(i1, i2 int) bool {
		return functionReports[i1].FuncName < functionReports[i2].FuncName
	})

	assert.Equal(t, fmt.Sprintf("%v/%v", gatherer.GetName(), "function_1"), functionReports[0].FuncName)
	assert.Equal(t, fmt.Sprintf("%v/%v", gatherer.GetName(), "function_2"), functionReports[1].FuncName)
	assert.Equal(t, fmt.Sprintf("%v/%v", gatherer.GetName(), "function_3"), functionReports[2].FuncName)

	// the execution is parallel so testing gets a little tricky
	totalRecordsCount := 0
	var totalErrs []string
	var totalWarnings []string
	for _, report := range functionReports {
		totalRecordsCount += report.RecordsCount
		totalErrs = append(totalErrs, report.Errors...)
		totalWarnings = append(totalWarnings, report.Warnings...)
		assert.Nil(t, report.Panic)
	}

	assert.Equal(t, 2, totalRecordsCount)
	assert.Len(t, totalErrs, 1)
	assert.Len(t, totalWarnings, 1)
	assert.Len(t, totalWarnings, 1)

	err = rec.Flush()
	assert.NoError(t, err)

	assert.Len(t, mockDriver.Saves, 1)
	records := mockDriver.Saves[0]
	assert.Len(t, records, 2)
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
		if strings.HasSuffix(rec.Name, recorder.MetadataRecordName) {
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

func assertRecordsOneGatherer(t testing.TB, records, expectedRecords []record.Record) {
	var recordsWithoutMetadata []record.Record
	for _, r := range records {
		if !strings.HasSuffix(r.Name, recorder.MetadataRecordName) {
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

// MockDriver implements a driver saving all the records to the Saves field
type MockDriver struct {
	Saves []record.MemoryRecords
}

func (md *MockDriver) Save(records record.MemoryRecords) (record.MemoryRecords, error) {
	md.Saves = append(md.Saves, records)
	return records, nil
}
func (*MockDriver) Prune(_ time.Time) error { return nil }
