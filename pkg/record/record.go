package record

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Represents a record that will be stored as a file.
type Record struct {
	Name     string
	Captured time.Time

	Fingerprint string
	Item        Marshalable
}

// Filename with extension, if present
func (r *Record) Filename() string {
	extension := r.Item.GetExtension()
	if len(extension) > 0 {
		return fmt.Sprintf("%s.%s", r.Name, extension)
	}
	return r.Name
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
