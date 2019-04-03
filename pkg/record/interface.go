package record

import (
	"context"
	"encoding/json"
	"time"
)

type Interface interface {
	Record(Record) error
}

type Record struct {
	Name     string
	Captured time.Time

	Fingerprint string
	Item        Marshalable
}

type Marshalable interface {
	Marshal(context.Context) ([]byte, error)
}

type JSONMarshaller struct {
	Object interface{}
}

func (m JSONMarshaller) Marshal(_ context.Context) ([]byte, error) {
	return json.Marshal(m.Object)
}
