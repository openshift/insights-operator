package marshal

// Raw is another simplification of marshaling from string
type Raw struct{ Str string }

// Marshal returns raw bytes
func (r Raw) Marshal() ([]byte, error) {
	return []byte(r.Str), nil
}

// GetExtension returns extension for raw marshaller
func (r Raw) GetExtension() string {
	return ""
}
