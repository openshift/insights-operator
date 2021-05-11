package recorder

import "github.com/openshift/insights-operator/pkg/record"

// MockRecorder records everything to the field Records
type MockRecorder struct {
	Records []record.Record
}

func (mr *MockRecorder) Record(r record.Record) error {
	mr.Records = append(mr.Records, r)
	return nil
}

func (*MockRecorder) Flush() error {
	return nil
}

func (mr *MockRecorder) Reset() {
	mr.Records = []record.Record{}
}
