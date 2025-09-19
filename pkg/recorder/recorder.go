package recorder

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/anonymization"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/types"
)

// MaxArchiveSize defines maximum allowed tarball size
const MaxArchiveSize = 24 * 1024 * 1024

// MetadataRecordName defines the metadata record name
const MetadataRecordName = "insights-operator/gathers"

type alreadyReported interface {
	LastReportedTime() time.Time
}

// Recorder struct
type Recorder struct {
	driver               Driver
	interval             time.Duration
	maxAge               time.Duration
	lock                 sync.Mutex
	size                 int64
	maxArchiveSize       int64
	records              map[string]*record.MemoryRecord
	recordedFingerprints map[string]string
	anonymizer           *anonymization.Anonymizer
}

// New recorder
func New(driver Driver, interval time.Duration, anonymizer *anonymization.Anonymizer) *Recorder {
	return &Recorder{
		driver:               driver,
		interval:             interval,
		maxArchiveSize:       MaxArchiveSize,
		maxAge:               interval * 6 * 24,
		records:              make(map[string]*record.MemoryRecord),
		recordedFingerprints: make(map[string]string),
		anonymizer:           anonymizer,
	}
}

// Record the report
func (r *Recorder) Record(rec record.Record) (errs []error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if rec.Item == nil {
		errs = append(errs, fmt.Errorf(`empty "%s" record data. Nothing will be recorded`, rec.Name))
		return errs
	}

	data, fingerprint, err := rec.Marshal()
	if err != nil {
		errs = append(errs, err)
		return errs
	}

	klog.Infof("Recording %s with fingerprint=%s", rec.Name, fingerprint)

	at := rec.Captured
	if at.IsZero() {
		at = time.Now()
	}

	recordName := rec.GetFilename()
	recordSize := int64(len(data))

	memoryRecord := &record.MemoryRecord{
		Name:        recordName,
		Fingerprint: fingerprint,
		At:          at,
		Data:        data,
	}

	if r.anonymizer != nil {
		memoryRecord, err = r.anonymizer.AnonymizeData(memoryRecord)
		if err != nil {
			return append(errs, err)
		}
	}

	// we want to record the "priority" files (with AlwaysStore=true) everytime regardless the archive size limit
	if r.size+recordSize > r.maxArchiveSize && !rec.AlwaysStored {
		errs = append(errs, fmt.Errorf(
			"record %s(size=%d) exceeds the archive size limit %d and will not be included in the archive",
			recordName, recordSize, r.maxArchiveSize,
		))
		return errs
	}

	if existingRecord, found := r.records[memoryRecord.Name]; found {
		errs = append(errs, fmt.Errorf(
			`the record with the same name "%v" was already recorded and had the fingerprint "%v", `+
				`overwriting with the record having fingerprint "%v"`,
			memoryRecord.Name, existingRecord.Fingerprint, memoryRecord.Fingerprint,
		))
		r.size -= int64(len(existingRecord.Data))
	}

	r.size += recordSize
	r.records[memoryRecord.Name] = memoryRecord

	if existingPath, found := r.recordedFingerprints[fingerprint]; found {
		existingRecord, found := r.records[existingPath]
		if !found {
			existingRecord = &record.MemoryRecord{Name: "unable to find the record"}
		}
		// this doesn't necessarily mean it's an error. There can be a collision after hashing
		errs = append(errs, &types.Warning{UnderlyingValue: fmt.Errorf(
			`the record with the same fingerprint "%v" was already recorded at path "%v", `+
				`recording another one with a different path "%v"`,
			fingerprint, existingRecord.Name, memoryRecord.Name,
		)})
	}

	r.recordedFingerprints[fingerprint] = recordName

	return errs
}

// Flush and save the reports using recorder driver
func (r *Recorder) Flush() error {
	defer r.storeTranslationTables()

	records := r.copy()
	if len(records) == 0 {
		return nil
	}

	sort.Sort(records)
	saved, err := r.driver.Save(records)
	defer func() {
		r.clear(saved)
	}()
	if err != nil {
		return err
	}

	return nil
}

func (r *Recorder) storeTranslationTables() {
	if r.anonymizer == nil {
		return
	}

	for _, anonymizer := range r.anonymizer.Anonymizers {
		if netAnonymizer, ok := anonymizer.(*anonymization.NetworkAnonymizer); ok {
			netAnonymizer.StoreTranslationTable()
		}
	}
}

// PeriodicallyPrune the reports using the recorder driver
func (r *Recorder) PeriodicallyPrune(ctx context.Context, reported alreadyReported) {
	wait.Until(func() {
		basePruneInterval := r.interval * 2
		interval := wait.Jitter(basePruneInterval, 1.2)
		klog.Infof("Pruning old reports every %s, max age is %s", interval.Truncate(time.Second), r.maxAge)
		timer := time.NewTicker(interval)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}

			_ = wait.ExponentialBackoff(wait.Backoff{Duration: time.Second, Steps: 4, Factor: 1.5}, func() (bool, error) {
				lastReported := reported.LastReportedTime()
				if oldestAllowed := time.Now().Add(-r.maxAge); lastReported.Before(oldestAllowed) {
					lastReported = oldestAllowed
				}

				if err := r.driver.Prune(lastReported); err != nil {
					klog.Errorf("Failed to prune older records: %v", err)
					return false, nil
				}
				return true, nil
			})
		}
	}, time.Second, ctx.Done())
}

func (r *Recorder) copy() record.MemoryRecords {
	r.lock.Lock()
	defer r.lock.Unlock()
	copies := make([]record.MemoryRecord, 0, len(r.records))
	for _, rec := range r.records {
		if rec.Data == nil {
			continue
		}
		copies = append(copies, *rec)
	}
	return copies
}

func (r *Recorder) clear(records record.MemoryRecords) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.records = make(map[string]*record.MemoryRecord, len(records))
	r.recordedFingerprints = make(map[string]string, len(records))
	r.size = 0
}
