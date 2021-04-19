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
	"strings"
	"time"

	"github.com/openshift/insights-operator/pkg/record"
	"k8s.io/klog/v2"
)

type DiskRecorder struct {
	basePath string
}

// New diskrecorder driver
func New(path string) *DiskRecorder {
	return &DiskRecorder{basePath: path}
}

// Save the records into the archive
func (d *DiskRecorder) Save(records record.MemoryRecords) (record.MemoryRecords, error) {
	wrote := 0
	start := time.Now()
	defer func() {
		if wrote > 0 {
			klog.V(2).Infof("Wrote %d records to disk in %s", wrote, time.Since(start).Truncate(time.Millisecond))
		}
	}()

	age := records[0].At.UTC()

	name := fmt.Sprintf("insights-%s.tar.gz", age.Format("2006-01-02-150405"))
	path := filepath.Join(d.basePath, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0640)
	if err != nil {
		if os.IsExist(err) {
			klog.Errorf("Tried to copy to %s which already exists", name)
			return nil, err
		}
		return nil, fmt.Errorf("unable to create archive: %v", err)
	}
	defer f.Close()

	completed := make([]record.MemoryRecord, 0, len(records))
	defer func() {
		wrote = len(completed)
	}()

	klog.V(4).Infof("Writing %d records to %s", len(records), path)

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	for _, record := range records {
		if err := tw.WriteHeader(&tar.Header{
			Name:     record.Name,
			ModTime:  record.At,
			Mode:     int64(os.FileMode(0640).Perm()),
			Size:     int64(len(record.Data)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			return nil, fmt.Errorf("unable to write tar header: %v", err)
		}
		if _, err := tw.Write(record.Data); err != nil {
			return nil, fmt.Errorf("unable to write tar entry: %v", err)
		}
		completed = append(completed, record)
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("unable to close tar writer: %v", err)
	}
	if err := gw.Close(); err != nil {
		return nil, fmt.Errorf("unable to close gzip writer: %v", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("unable to close file: %v", err)
	}
	return completed, nil
}

// Prune the archives older than given time
func (d *DiskRecorder) Prune(olderThan time.Time) error {
	files, err := ioutil.ReadDir(d.basePath)
	if err != nil {
		return err
	}
	count := 0
	var errors []string
	for _, file := range files {
		if isNotArchiveFile(file) {
			continue
		}
		if file.ModTime().After(olderThan) {
			continue
		}
		if err := os.Remove(filepath.Join(d.basePath, file.Name())); err != nil {
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

// Summary implements summarizer interface to insights uploader
func (d *DiskRecorder) Summary(_ context.Context, since time.Time) (io.ReadCloser, bool, error) {
	files, err := ioutil.ReadDir(d.basePath)
	if err != nil {
		return nil, false, err
	}
	if len(files) == 0 {
		return nil, false, nil
	}
	recentFiles := make([]string, 0, len(files))
	for _, file := range files {
		if isNotArchiveFile(file) {
			continue
		}
		if !file.ModTime().After(since) {
			continue
		}
		recentFiles = append(recentFiles, file.Name())
	}
	if len(recentFiles) == 0 {
		return nil, false, nil
	}
	lastFile := recentFiles[len(recentFiles)-1]
	klog.V(4).Infof("Found files to send: %v", lastFile)
	f, err := os.Open(filepath.Join(d.basePath, lastFile))
	if err != nil {
		return nil, false, nil
	}
	return f, true, nil
}

func isNotArchiveFile(file os.FileInfo) bool {
	return file.IsDir() || !strings.HasPrefix(file.Name(), "insights-") || !strings.HasSuffix(file.Name(), ".tar.gz")
}
