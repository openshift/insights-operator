package recorder

import (
	"context"
	"sort"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
)

// MaxLogSize defines maximum allowed tarball size
const MaxLogSize = 8 * 1024 * 1024

type alreadyReported interface {
	LastReportedTime() time.Time
}

// Recorder struct
type Recorder struct {
	driver    Driver
	flushCh   chan struct{}
	flushSize int64 // defines maximum allowed report size
	interval  time.Duration
	maxAge    time.Duration
	lock      sync.Mutex
	size      int64
	records   map[string]*record.MemoryRecord
}

// New recorder
func New(driver Driver, interval time.Duration) *Recorder {
	return &Recorder{
		driver:    driver,
		interval:  interval,
		maxAge:    interval * 6 * 24,
		records:   make(map[string]*record.MemoryRecord),
		flushCh:   make(chan struct{}, 1),
		flushSize: MaxLogSize,
	}
}

// Record the report
func (r *Recorder) Record(rec record.Record) error {
	r.lock.Lock()
	defer r.lock.Unlock()
	klog.V(4).Infof("Recording %s with fingerprint=%s", rec.Name, rec.Fingerprint)
	if r.has(rec) {
		return nil
	}

	at := rec.Captured
	if at.IsZero() {
		at = time.Now()
	}

	// TODO: handle records that are slow to capture
	data, err := rec.Item.Marshal(context.TODO())
	if err != nil {
		return err
	}

	recordName := rec.Filename()
	r.records[recordName] = &record.MemoryRecord{
		Name:        recordName,
		Fingerprint: rec.Fingerprint,
		At:          at,
		Data:        data,
	}
	r.size += int64(len(data))

	// trigger a flush if we're above our threshold
	if r.size > r.flushSize {
		select {
		case r.flushCh <- struct{}{}:
		default:
		}
	}

	return nil
}

// Flush and save the reports using recorder driver
func (r *Recorder) Flush() error {
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

// PeriodicallyPrune the reports using the recorder driver
func (r *Recorder) PeriodicallyPrune(ctx context.Context, reported alreadyReported) {
	wait.Until(func() {
		interval := wait.Jitter(r.interval*2, 1.2)
		klog.V(2).Infof("Pruning old reports every %s, max age is %s", interval.Truncate(time.Second), r.maxAge)
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

func (r *Recorder) has(record record.Record) bool {
	existing, ok := r.records[record.Filename()]
	if ok {
		if len(record.Fingerprint) > 0 && record.Fingerprint == existing.Fingerprint {
			return true
		}
	}
	return false
}

func (r *Recorder) copy() record.MemoryRecords {
	r.lock.Lock()
	defer r.lock.Unlock()
	copies := make([]record.MemoryRecord, 0, len(r.records))
	for _, record := range r.records {
		if record.Data == nil {
			continue
		}
		copies = append(copies, *record)
	}
	return copies
}

func (r *Recorder) clear(records record.MemoryRecords) {
	r.lock.Lock()
	defer r.lock.Unlock()
	size := int64(0)
	for _, record := range records {
		existing, ok := r.records[record.Name]
		if !ok || existing.Data == nil || existing.At != record.At || existing.Fingerprint != record.Fingerprint {
			continue
		}
		size += int64(len(existing.Data))
		existing.Data = nil
	}
	r.size -= size
}
