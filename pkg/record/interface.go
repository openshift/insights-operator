package record

import (
	"context"
	"encoding/json"
	"time"
)

type Interface interface {
	Record(Record) error
}

type FlushInterface interface {
	Interface
	Flush(context.Context) error
}

type Record struct {
	Name     string
	Captured time.Time

	Fingerprint string
	Item        Marshalable
}

type Marshalable interface {
	Marshal(context.Context) ([]byte, error)
	GetExtension() string
}

type JSONMarshaller struct {
	Object interface{}
}

func (m JSONMarshaller) Marshal(_ context.Context) ([]byte, error) {
	return json.Marshal(m.Object)
}

// GetExtension return extension for json marshaller
func (m JSONMarshaller) GetExtension() string {
	return "json"
}
