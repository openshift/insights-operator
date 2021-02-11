package tests

import (
	"context"
	"encoding/json"
)

// RawReport implements Marshable interface
type RawReport struct{ Data string }

// Marshal returns raw bytes
func (r RawReport) Marshal(_ context.Context) ([]byte, error) {
	return []byte(r.Data), nil
}

// GetExtension returns extension for raw marshaller
func (r RawReport) GetExtension() string {
	return ""
}

// RawInvalidReport implements Marshable interface but throws an error
type RawInvalidReport struct{}

// Marshal returns raw bytes
func (r RawInvalidReport) Marshal(_ context.Context) ([]byte, error) {
	return nil, &json.UnsupportedTypeError{}
}

// GetExtension returns extension for raw marshaller
func (r RawInvalidReport) GetExtension() string {
	return ""
}
