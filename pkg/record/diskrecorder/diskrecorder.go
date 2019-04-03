package diskrecorder

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"k8s.io/klog"

	"github.com/openshift/support-operator/pkg/record"
)

type memoryRecord struct {
	name        string
	fingerprint string
	at          time.Time
	data        []byte
}

type memoryRecords []memoryRecord

func (r memoryRecords) Less(i, j int) bool { return r[i].name < r[j].name }
func (r memoryRecords) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
func (r memoryRecords) Len() int           { return len(r) }

type Recorder struct {
	basePath  string
	flushCh   chan struct{}
	flushSize int64
	interval  time.Duration
	maxAge    time.Duration

	lock    sync.Mutex
	size    int64
	records map[string]*memoryRecord
}

func New(basePath string, interval time.Duration) *Recorder {
	return &Recorder{
		basePath:  basePath,
		interval:  interval,
		maxAge:    interval * 6 * 24,
		records:   make(map[string]*memoryRecord),
		flushCh:   make(chan struct{}, 1),
		flushSize: 8 * 1024 * 1024,
	}
}

func (r *Recorder) Record(record record.Record) error {
	r.lock.Lock()
	defer r.lock.Unlock()
	klog.V(4).Infof("Recording %s with fingerprint=%s", record.Name, record.Fingerprint)
	existing, ok := r.records[record.Name]
	if ok {
		if len(record.Fingerprint) > 0 && record.Fingerprint == existing.fingerprint {
			return nil
		}
	}

	at := record.Captured
	if at.IsZero() {
		at = time.Now()
	}

	// TODO: handle records that are slow to capture
	data, err := record.Item.Marshal(context.TODO())
	if err != nil {
		return err
	}

	r.records[record.Name] = &memoryRecord{
		name:        record.Name,
		fingerprint: record.Fingerprint,
		at:          at,
		data:        data,
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

func (r *Recorder) copyRecords() memoryRecords {
	r.lock.Lock()
	defer r.lock.Unlock()
	copies := make([]memoryRecord, 0, len(r.records))
	for _, record := range r.records {
		if record.data == nil {
			continue
		}
		copies = append(copies, *record)
	}
	return copies
}

func (r *Recorder) clearRecords(records memoryRecords) {
	r.lock.Lock()
	defer r.lock.Unlock()
	size := int64(0)
	for _, record := range records {
		existing, ok := r.records[record.name]
		if !ok || existing.data == nil || existing.at != record.at || existing.fingerprint != record.fingerprint {
			continue
		}
		size += int64(len(existing.data))
		existing.data = nil
	}
	r.size -= size
}

func (r *Recorder) PeriodicallyFlush(ctx context.Context) {
	wait.Until(func() {
		timer := time.NewTicker(r.interval)
		defer timer.Stop()
		for {

			select {
			case <-ctx.Done():
				if err := r.Flush(ctx); err != nil {
					records := r.copyRecords()
					klog.Errorf("Unable to write records on shutdown, exiting with %d records not flushed to disk: %v", len(records), err)
				}
				return
			case <-timer.C:
			case <-r.flushCh:
			}

			wait.ExponentialBackoff(wait.Backoff{Duration: time.Second, Steps: 4, Factor: 1.5}, func() (bool, error) {
				if err := r.Flush(ctx); err != nil {
					klog.Errorf("Failed to flush records to disk: %v", err)
					return false, nil
				}
				return true, nil
			})
		}

	}, time.Second, ctx.Done())
}

func (r *Recorder) Flush(ctx context.Context) error {
	wrote := 0
	start := time.Now()
	defer func() {
		if wrote > 0 {
			klog.V(2).Infof("Wrote %d records to disk in %s", wrote, time.Now().Sub(start).Truncate(time.Millisecond))
		}
	}()

	records := r.copyRecords()
	if len(records) == 0 {
		return nil
	}

	sort.Sort(records)
	var age time.Time
	for _, record := range records {
		if record.at.After(age) {
			age = record.at
		}
	}
	age = age.UTC()

	name := fmt.Sprintf("support-%s.tar.gz", age.Format("2006-01-02-150405"))
	path := filepath.Join(r.basePath, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0640)
	if err != nil {
		if os.IsExist(err) {
			klog.Errorf("Tried to copy to %s which already exists", name)
			return nil
		}
		return fmt.Errorf("unable to create archive: %v", err)
	}
	defer f.Close()

	completed := make([]memoryRecord, 0, len(records))
	defer func() {
		wrote = len(completed)
		r.clearRecords(completed)
	}()

	klog.V(4).Infof("Writing %d records to %s", len(records), path)

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	done := ctx.Done()
	for _, record := range records {
		select {
		case <-done:
			return fmt.Errorf("cancelled before all results could be written to disk")
		default:
		}

		if err := tw.WriteHeader(&tar.Header{
			Name:     record.name,
			ModTime:  record.at,
			Mode:     int64(os.FileMode(0640).Perm()),
			Size:     int64(len(record.data)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			return fmt.Errorf("unable to write tar header: %v", err)
		}
		if _, err := tw.Write(record.data); err != nil {
			return fmt.Errorf("unable to write tar entry: %v", err)
		}
		completed = append(completed, record)
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("unable to close tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		return fmt.Errorf("unable to close gzip writer: %v", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("unable to close file: %v", err)
	}
	return nil
}

type AlreadyReported interface {
	LastReportedTime() time.Time
}

func (r *Recorder) PeriodicallyPrune(ctx context.Context, reported AlreadyReported) {
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

			wait.ExponentialBackoff(wait.Backoff{Duration: time.Second, Steps: 4, Factor: 1.5}, func() (bool, error) {
				lastReported := reported.LastReportedTime()
				if oldestAllowed := time.Now().Add(-r.maxAge); lastReported.Before(oldestAllowed) {
					lastReported = oldestAllowed
				}

				if err := r.Prune(ctx, lastReported); err != nil {
					klog.Errorf("Failed to prune older records: %v", err)
					return false, nil
				}
				return true, nil
			})
		}
	}, time.Second, ctx.Done())
}

func (r *Recorder) Prune(ctx context.Context, olderThan time.Time) error {
	files, err := ioutil.ReadDir(r.basePath)
	if err != nil {
		return err
	}
	count := 0
	var errors []string
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if file.ModTime().After(olderThan) {
			continue
		}
		if file.IsDir() || !strings.HasPrefix(file.Name(), "support-") || !strings.HasSuffix(file.Name(), ".tar.gz") {
			continue
		}
		if err := os.Remove(filepath.Join(r.basePath, file.Name())); err != nil {
			errors = append(errors, err.Error())
			continue
		}
		count++
	}
	if len(errors) == 1 {
		return fmt.Errorf("failed to delete expired file: %v", errors[0])
	}
	if len(errors) > 1 {
		return fmt.Errorf("failed to delete %d expired files: %v", len(errors), errors[0])
	}
	if count > 0 {
		klog.V(4).Infof("Deleted %d files older than %s", count, olderThan.UTC().Format(time.RFC3339))
	}
	return nil
}

func (r *Recorder) Summary(ctx context.Context, since time.Time) (io.ReadCloser, bool, error) {
	files, err := ioutil.ReadDir(r.basePath)
	if err != nil {
		return nil, false, err
	}

	var merged []string
	for i, file := range files {
		if file.IsDir() {
			continue
		}
		if !file.ModTime().After(since) {
			continue
		}
		merged = make([]string, 0, len(files)-i)
		for j := len(files) - 1; j >= i; j-- {
			if files[j].IsDir() || !strings.HasPrefix(files[j].Name(), "support-") || !strings.HasSuffix(files[j].Name(), ".tar.gz") {
				continue
			}
			merged = append(merged, filepath.Join(r.basePath, files[j].Name()))
		}
	}
	if len(merged) == 0 {
		return nil, false, nil
	}
	klog.V(4).Infof("Found files to send: %v", merged)

	return &mergeReader{filenames: merged}, true, nil
}

type mergeReader struct {
	filenames []string
	pw        *io.PipeWriter
	pr        *io.PipeReader
}

func (r *mergeReader) Read(p []byte) (int, error) {
	if r.pw == nil {
		r.pr, r.pw = io.Pipe()
		go r.merge()
	}
	return r.pr.Read(p)
}

func (r *mergeReader) Close() error {
	if r.pw != nil {
		return r.pw.Close()
	}
	return nil
}

func (r *mergeReader) merge() {
	gw := gzip.NewWriter(r.pw)
	tw := tar.NewWriter(gw)

	types := make(map[string]struct{})
	for _, filename := range r.filenames {
		err := func() error {
			f, err := os.Open(filename)
			if err != nil {
				return err
			}
			defer f.Close()
			gr, err := gzip.NewReader(f)
			if err != nil {
				return err
			}
			tr := tar.NewReader(gr)
			for {
				hdr, err := tr.Next()
				if err != nil {
					if err == io.EOF {
						return nil
					}
					return err
				}
				if _, ok := types[hdr.Name]; ok {
					continue
				}
				if err := tw.WriteHeader(hdr); err != nil {
					return err
				}
				if _, err := io.Copy(tw, tr); err != nil {
					return err
				}
			}
		}()
		if err != nil {
			r.pw.CloseWithError(err)
			return
		}
	}
	if err := tw.Close(); err != nil {
		r.pw.CloseWithError(err)
		return
	}
	r.pw.CloseWithError(gw.Close())
}
