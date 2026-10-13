package marshal

// RawByte is skipping marshaling from byte slice
type RawByte []byte

// Marshal just returns bytes
func (r RawByte) Marshal() ([]byte, error) {
	return r, nil
}

// GetExtension returns extension for "id" file - none
func (r RawByte) GetExtension() string {
	return ""
}
