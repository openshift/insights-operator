package recorder

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/openshift/api/insights/v1alpha2"
	"github.com/openshift/insights-operator/pkg/anonymization"
	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/record"
)

const mock1Name = "config/mock1"

// RawReport implements Marshable interface
type RawReport struct{ Data string }

// Marshal returns raw bytes
func (r RawReport) Marshal() ([]byte, error) {
	return []byte(r.Data), nil
}

// GetExtension returns extension for raw marshaller
func (r RawReport) GetExtension() string {
	return ""
}

// RawInvalidReport implements Marshable interface but throws an error
type RawInvalidReport struct{}

// Marshal returns raw bytes
func (r RawInvalidReport) Marshal() ([]byte, error) {
	return nil, &json.UnsupportedTypeError{}
}

// GetExtension returns extension for raw marshaller
func (r RawInvalidReport) GetExtension() string {
	return ""
}

type driverMock struct {
	mock.Mock
}

func (d *driverMock) Save(records record.MemoryRecords) (record.MemoryRecords, error) {
	args := d.Called()
	return records, args.Error(1)
}

func (d *driverMock) Prune(time.Time) error {
	args := d.Called()
	return args.Error(1)
}

func newRecorder(maxArchiveSize int64, clusterBaseDomain string) (*Recorder, error) {
	driver := driverMock{}
	driver.On("Save").Return(nil, nil)
	mockConfigMapConfigurator := config.NewMockConfigMapConfigurator(&config.InsightsConfiguration{
		DataReporting: config.DataReporting{
			Obfuscation: config.Obfuscation{
				config.Networking,
			},
		},
	})

	networkAnonymizationBuilder := &anonymization.NetworkAnonymizerBuilder{}
	networkAnonymizer, err := networkAnonymizationBuilder.WithSensitiveValue(clusterBaseDomain, anonymization.ClusterBaseDomainPlaceholder).
		WithDataPolicies(v1alpha2.DataPolicyOptionObfuscateNetworking).
		WithConfigurator(mockConfigMapConfigurator).
		Build()
	if err != nil {
		return nil, err
	}

	anonymizer, err := anonymization.NewAnonymizer(networkAnonymizer)
	if err != nil {
		return nil, err
	}

	interval, _ := time.ParseDuration("1m")
	return &Recorder{
		driver:               &driver,
		interval:             interval,
		maxAge:               interval * 6 * 24,
		maxArchiveSize:       maxArchiveSize,
		records:              make(map[string]*record.MemoryRecord),
		recordedFingerprints: make(map[string]string),
		anonymizer:           anonymizer,
	}, nil
}

func Test_Record(t *testing.T) {
	rec, err := newRecorder(MaxArchiveSize, "")
	assert.NoError(t, err)
	errs := rec.Record(record.Record{
		Name: mock1Name,
		Item: RawReport{Data: "mock1"},
	})
	assert.Empty(t, errs)
	assert.Equal(t, 1, len(rec.records))
}

func Test_Record_Duplicated(t *testing.T) {
	rec, err := newRecorder(MaxArchiveSize, "")
	assert.NoError(t, err)
	errs := rec.Record(record.Record{
		Name: mock1Name,
		Item: RawReport{Data: "mock1"},
	})
	assert.Empty(t, errs)
	errs = rec.Record(record.Record{
		Name: mock1Name,
		Item: RawReport{Data: "mock1"},
	})
	assert.Len(t, errs, 2)
	assert.Equal(t, 1, len(rec.records))
}

func Test_Record_CantBeSerialized(t *testing.T) {
	rec, err := newRecorder(MaxArchiveSize, "")
	assert.NoError(t, err)
	errs := rec.Record(record.Record{
		Name: mock1Name,
		Item: RawInvalidReport{},
	})
	assert.Len(t, errs, 1)
	assert.Error(t, errs[0])
}

func Test_Record_Flush(t *testing.T) {
	rec, err := newRecorder(MaxArchiveSize, "")
	assert.NoError(t, err)
	for i := 0; i < 3; i++ {
		errs := rec.Record(record.Record{
			Name: fmt.Sprintf("config/mock%d", i),
			Item: RawReport{Data: "mockdata"},
		})
		if i > 0 {
			assert.NotEmpty(t, errs)
		} else {
			assert.Empty(t, errs)
		}
	}
	err = rec.Flush()
	assert.NoError(t, err)
	assert.Equal(t, int64(0), rec.size)
}

func Test_Record_FlushEmptyRecorder(t *testing.T) {
	rec, err := newRecorder(MaxArchiveSize, "")
	assert.NoError(t, err)
	err = rec.Flush()
	assert.NoError(t, err)
}

func Test_Record_ArchiveSizeExceeded(t *testing.T) {
	data := "data bigger than 4 bytes"
	maxArchiveSize := int64(4)
	rec, err := newRecorder(maxArchiveSize, "")
	assert.NoError(t, err)
	errs := rec.Record(record.Record{
		Name: mock1Name,
		Item: RawReport{
			Data: data,
		},
	})
	assert.Len(t, errs, 1)
	err = errs[0]
	assert.Equal(
		t,
		err,
		fmt.Errorf(
			"record %s(size=%d) exceeds the archive size limit %d and will not be included in the archive",
			mock1Name,
			len([]byte(data)),
			maxArchiveSize,
		),
	)
}

func Test_Record_SizeDoesntGrowWithSameRecords(t *testing.T) {
	data := "testdata"
	testRec := record.Record{
		Name: mock1Name,
		Item: RawReport{
			Data: data,
		},
	}
	rec, err := newRecorder(MaxArchiveSize, "")
	assert.NoError(t, err)
	errs := rec.Record(testRec)
	assert.Empty(t, errs)
	// record again the same record
	errs = rec.Record(testRec)
	assert.Len(t, errs, 2)

	// check that size refers only to one record data
	assert.Equal(t, rec.size, int64(len(data)))
	err = rec.Flush()
	assert.Nil(t, err)
	assert.Equal(t, rec.size, int64(0))
}

func Test_ObfuscatedRecord_NameCorrect(t *testing.T) {
	clusterBaseDomain := "test"
	testRecordName := fmt.Sprintf("%s/%s-node-1", mock1Name, clusterBaseDomain)
	obfuscatedRecordName := fmt.Sprintf("%s/%s-node-1", mock1Name, anonymization.ClusterBaseDomainPlaceholder)
	rec, err := newRecorder(MaxArchiveSize, clusterBaseDomain)
	assert.NoError(t, err)
	errs := rec.Record(record.Record{
		Name: testRecordName,
		Item: RawReport{
			Data: "some data",
		},
	})
	assert.Empty(t, errs)
	_, exists := rec.records[obfuscatedRecordName]
	assert.True(t, exists, "can't find %s record name", testRecordName)
	err = rec.Flush()
	assert.Nil(t, err)
	assert.Equal(t, rec.size, int64(0))
}

func Test_EmptyItemRecord(t *testing.T) {
	rec, err := newRecorder(MaxArchiveSize, "")
	assert.NoError(t, err)
	testRec := record.Record{
		Name: "test/empty",
	}
	errs := rec.Record(testRec)
	assert.Len(t, errs, 1)
	err = errs[0]
	assert.Equal(t, fmt.Errorf(`empty "%s" record data. Nothing will be recorded`, testRec.Name), err)
}
