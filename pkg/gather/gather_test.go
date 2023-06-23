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

	"github.com/openshift/api/config/v1alpha1"
	"github.com/openshift/insights-operator/pkg/anonymization"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/recorder"
	"github.com/openshift/insights-operator/pkg/types"
)

func Test_getEnabledGatheringFunctions(t *testing.T) {
	tests := []struct {
		testName     string
		gathererName string
		all          map[string]gatherers.GatheringClosure
		disabled     []string
		expected     map[string]gatherers.GatheringClosure
	}{
		{
			testName:     "disable some functions",
			gathererName: "clusterconfig",
			all: map[string]gatherers.GatheringClosure{
				"container_images": {},
				"nodes":            {},
				"authentication":   {},
				"some_function":    {},
			},
			disabled: []string{
				"clusterconfig/container_images",
				"clusterconfig/nodes",
			},
			expected: map[string]gatherers.GatheringClosure{
				"authentication": {},
				"some_function":  {},
			},
		},
		{
			testName:     "disable non-existing functions",
			gathererName: "clusterconfig",
			all: map[string]gatherers.GatheringClosure{
				"container_images": {},
				"nodes":            {},
				"authentication":   {},
				"some_function":    {},
			},
			disabled: []string{
				"clusterconfig/foo",
				"clusterconfig/bar",
			},
			expected: map[string]gatherers.GatheringClosure{
				"container_images": {},
				"nodes":            {},
				"authentication":   {},
				"some_function":    {},
			},
		},
		{
			testName:     "disable complete top-level gatherer",
			gathererName: "clusterconfig",
			all: map[string]gatherers.GatheringClosure{
				"container_images": {},
				"nodes":            {},
				"authentication":   {},
				"some_function":    {},
			},
			disabled: []string{
				"clusterconfig",
			},
			expected: map[string]gatherers.GatheringClosure{},
		},
		{
			testName:     "no functions disabled",
			gathererName: "clusterconfig",
			all: map[string]gatherers.GatheringClosure{
				"container_images": {},
				"nodes":            {},
				"authentication":   {},
				"some_function":    {},
			},
			disabled: []string{},
			expected: map[string]gatherers.GatheringClosure{
				"container_images": {},
				"nodes":            {},
				"authentication":   {},
				"some_function":    {},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			result := getEnabledGatheringFunctions(tt.gathererName, tt.all, tt.disabled)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// nolint: funlen
func Test_StartGatheringConcurrently(t *testing.T) {
	gatherer := &MockGatherer{SomeField: "some_value"}

	resultsChan, err := startGatheringConcurrently(context.Background(), gatherer, nil)
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

	resultsChan, err = startGatheringConcurrently(context.Background(), gatherer, &v1alpha1.GatherConfig{
		DisabledGatherers: []string{
			"mock_gatherer/3_records",
			"mock_gatherer/errors",
			"mock_gatherer/panic",
			"mock_gatherer/name",
		},
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

	resultsChan, err = startGatheringConcurrently(context.Background(), gatherer, &v1alpha1.GatherConfig{
		DisabledGatherers: []string{
			"mock_gatherer/some_field",
			"mock_gatherer/errors",
			"mock_gatherer/panic",
		},
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

	resultsChan, err := startGatheringConcurrently(context.Background(), gatherer, &v1alpha1.GatherConfig{
		DisabledGatherers: []string{
			"mock_gatherer/some_field",
			"mock_gatherer/errors",
			"mock_gatherer/panic",
			"mock_gatherer/name",
			"mock_gatherer/3_records",
		},
	})
	assert.EqualError(t, err, "no gather functions are specified to run")
	assert.Nil(t, resultsChan)

	resultsChan, err = startGatheringConcurrently(context.Background(), gatherer, &v1alpha1.GatherConfig{
		DisabledGatherers: []string{
			"mock_gatherer",
		},
	})
	assert.EqualError(t, err, "no gather functions are specified to run")
	assert.Nil(t, resultsChan)
}

func Test_CollectAndRecordGatherer(t *testing.T) {
	gatherer := &MockGatherer{
		SomeField: "some_value",
	}
	mockRecorder := &recorder.MockRecorder{}
	mockAPIConfigurator := config.NewMockAPIConfigurator(&v1alpha1.GatherConfig{})
	mockSecretConfigurator := config.NewMockSecretConfigurator(&config.Controller{
		EnableGlobalObfuscation: true,
	})
	anonymizer, err := anonymization.NewAnonymizer("", nil, nil, mockSecretConfigurator, mockAPIConfigurator)
	assert.NoError(t, err)

	functionReports, err := CollectAndRecordGatherer(context.Background(), gatherer, mockRecorder, mockAPIConfigurator.GatherConfig())
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
			Errors:   []string{"panic: test panic"},
			Panic:    "test panic",
		},
		{
			FuncName:     "mock_gatherer",
			RecordsCount: 5,
			Errors: []string{
				`function "errors" failed with an error`,
				`function "errors" failed with an error`,
				`function "errors" failed with an error`,
				`function "panic" panicked`,
			},
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
	mockAPIConfigurator := config.NewMockAPIConfigurator(&v1alpha1.GatherConfig{
		DisabledGatherers: []string{
			"mock_gatherer/some_field",
			"mock_gatherer/name",
			"mock_gatherer/panic",
			"mock_gatherer/3_records",
		},
	})

	functionReports, err := CollectAndRecordGatherer(context.Background(), gatherer, mockRecorder, mockAPIConfigurator.GatherConfig())
	assert.EqualError(
		t,
		err,
		`function "errors" failed with an error`,
	)
	anonymizer, err := anonymization.NewAnonymizer("", []string{}, nil, config.NewMockSecretConfigurator(nil), mockAPIConfigurator)
	assert.NoError(t, err)
	err = RecordArchiveMetadata(functionReports, mockRecorder, anonymizer)
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
		{
			FuncName:     "mock_gatherer",
			RecordsCount: 0,
			Errors: []string{
				`function "errors" failed with an error`,
				`function "errors" failed with an error`,
				`function "errors" failed with an error`,
			},
		},
	})
}

func Test_CollectAndRecordGatherer_Panic(t *testing.T) {
	gatherer := &MockGatherer{}
	mockRecorder := &recorder.MockRecorder{}
	mockAPIConfigurator := config.NewMockAPIConfigurator(&v1alpha1.GatherConfig{
		DisabledGatherers: []string{
			"mock_gatherer/some_field",
			"mock_gatherer/name",
			"mock_gatherer/errors",
			"mock_gatherer/3_records",
		},
	})

	functionReports, err := CollectAndRecordGatherer(context.Background(), gatherer, mockRecorder, mockAPIConfigurator.GatherConfig())
	assert.EqualError(t, err, `function "panic" panicked`)
	assert.Len(t, functionReports, 2)
	functionReports[0].Duration = 0
	functionReports[1].Duration = 0
	assert.ElementsMatch(t, functionReports, []GathererFunctionReport{
		{
			FuncName: "mock_gatherer/panic",
			Errors:   []string{"panic: test panic"},
			Panic:    "test panic",
		},
		{
			FuncName: "mock_gatherer",
			Errors:   []string{`function "panic" panicked`},
		},
	})
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
	mockAPIConfigurator := config.NewMockAPIConfigurator(&v1alpha1.GatherConfig{})
	anonymizer, err := anonymization.NewAnonymizer("", []string{}, nil, config.NewMockSecretConfigurator(nil), mockAPIConfigurator)
	assert.NoError(t, err)
	rec := recorder.New(mockDriver, time.Second, anonymizer)

	functionReports, err := CollectAndRecordGatherer(context.Background(), gatherer, rec, mockAPIConfigurator.GatherConfig())
	assert.Error(t, err)
	assert.NotEmpty(t, functionReports)
	assert.Len(t, functionReports, 4)

	sort.Slice(functionReports, func(i1, i2 int) bool {
		return functionReports[i1].FuncName < functionReports[i2].FuncName
	})

	assert.Equal(t, gatherer.GetName(), functionReports[0].FuncName)
	assert.Equal(t, fmt.Sprintf("%v/%v", gatherer.GetName(), "function_1"), functionReports[1].FuncName)
	assert.Equal(t, fmt.Sprintf("%v/%v", gatherer.GetName(), "function_2"), functionReports[2].FuncName)
	assert.Equal(t, fmt.Sprintf("%v/%v", gatherer.GetName(), "function_3"), functionReports[3].FuncName)

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

	assert.Equal(t, 4, totalRecordsCount)
	assert.Len(t, totalErrs, 2)
	assert.Len(t, totalWarnings, 1)
	assert.Len(t, totalWarnings, 1)

	err = rec.Flush()
	assert.NoError(t, err)

	assert.Len(t, mockDriver.Saves, 1)
	records := mockDriver.Saves[0]
	assert.Len(t, records, 2)
}

func Test_CollectAndRecordGatherer_Warning(t *testing.T) {
	gatherer := &MockGathererWithProvidedFunctions{Functions: map[string]gatherers.GatheringClosure{
		"function_1": {Run: func(ctx context.Context) ([]record.Record, []error) {
			return nil, []error{&types.Warning{UnderlyingValue: fmt.Errorf("test warning")}}
		}},
	}}
	mockDriver := &MockDriver{}
	rec := recorder.New(mockDriver, time.Second, nil)
	mockAPIConfigurator := config.NewMockAPIConfigurator(nil)

	functionReports, err := CollectAndRecordGatherer(context.Background(), gatherer, rec, mockAPIConfigurator.GatherConfig())
	assert.NoError(t, err)
	assert.Len(t, functionReports, 2)
	assert.Equal(t, "mock_gatherer_with_provided_functions/function_1", functionReports[0].FuncName)
	assert.Equal(t, 0, functionReports[0].RecordsCount)
	assert.Nil(t, functionReports[0].Errors)
	assert.Equal(t, []string{"warning: test warning"}, functionReports[0].Warnings)
	assert.Nil(t, functionReports[0].Panic)
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
			if len(metadataBytes) > 0 {
				t.Fatalf("found 2 metadata records")
			}

			bytes, err := rec.Item.Marshal()
			assert.NoError(t, err)

			metadataBytes = bytes
		}
	}

	var archiveMetadata ArchiveMetadata

	err := json.Unmarshal(metadataBytes, &archiveMetadata)
	assert.NoError(t, err)

	for i := range archiveMetadata.StatusReports {
		statusReport := &archiveMetadata.StatusReports[i]
		statusReport.Duration = 0
		sort.Slice(statusReport.Errors, func(i1, i2 int) bool {
			return statusReport.Errors[i1] < statusReport.Errors[i2]
		})
		sort.Slice(statusReport.Warnings, func(i1, i2 int) bool {
			return statusReport.Warnings[i1] < statusReport.Warnings[i2]
		})
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
