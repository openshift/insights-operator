package record

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

const (
	JSONExtension = "json"
)

// Record represents a record that will be stored as a file.
type Record struct {
	Name     string
	Captured time.Time
	Item     Marshalable
}

// Marshal marshals the item and returns its fingerprint
func (r *Record) Marshal() ([]byte, string, error) {
	content, err := r.Item.Marshal()
	if err != nil {
		return content, "", err
	}

	h := sha256.New()
	h.Write(content)
	fingerprint := hex.EncodeToString(h.Sum(nil))

	return content, fingerprint, nil
}

// GetFilename with extension, if present
func (r *Record) GetFilename() string {
	extension := r.Item.GetExtension()
	if len(extension) > 0 {
		return fmt.Sprintf("%s.%s", r.Name, extension)
	}
	return r.Name
}

type Marshalable interface {
	Marshal() ([]byte, error)
	GetExtension() string
}

type JSONMarshaller struct {
	Object interface{}
}

func (m JSONMarshaller) Marshal() ([]byte, error) {
	return json.Marshal(m.Object)
}

// GetExtension return extension for json marshaller
func (m JSONMarshaller) GetExtension() string {
	return JSONExtension
}
