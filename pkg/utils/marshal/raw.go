package marshal

import "context"

// Raw is another simplification of marshalling from string
type Raw struct{ Str string }

// Marshal returns raw bytes
func (r Raw) Marshal(_ context.Context) ([]byte, error) {
	return []byte(r.Str), nil
}

// GetExtension returns extension for raw marshaller
func (r Raw) GetExtension() string {
	return ""
}
