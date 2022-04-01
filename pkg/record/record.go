package record

import (
	"context"
	"crypto/md5"
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

	fingerprint string
}

// GetFingerprint returns the fingerprint possibly using the cache
func (r *Record) GetFingerprint(ctx context.Context) (string, error) {
	if len(r.fingerprint) == 0 {
		content, err := r.Item.Marshal(ctx)
		if err != nil {
			return "", err
		}

		// TODO: use a better algorithm? md5 can produce a lot of collisions
		h := md5.New()
		h.Write(content)
		r.fingerprint = hex.EncodeToString(h.Sum(nil))
	}

	return r.fingerprint, nil
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
	return JSONExtension
}
