package record

import (
	"fmt"
	"time"
)

// MemoryRecord Represents records stored in memory
type MemoryRecord struct {
	Name        string
	At          time.Time
	Data        []byte
	Fingerprint string
}

func (r *MemoryRecord) Print() string {
	return fmt.Sprintf(
		`MemoryRecord{Name: "%v", At: "%v", len(Data): %v, Fingerprint: "%v"}`,
		r.Name, r.At, len(r.Data), r.Fingerprint,
	)
}

type MemoryRecords []MemoryRecord

func (r MemoryRecords) Less(i, j int) bool { return r[i].At.After(r[j].At) }
func (r MemoryRecords) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
func (r MemoryRecords) Len() int           { return len(r) }
