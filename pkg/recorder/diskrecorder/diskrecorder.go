package diskrecorder

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
	"github.com/openshift/insights-operator/pkg/record"
)

type DiskRecorder struct {
	basePath      string
	lastRecording time.Time
}

// New diskrecorder driver
func New(path string) *DiskRecorder {
	return &DiskRecorder{basePath: path}
}

const archiveExtension = ".tar.gz"

// Save the records into the archive in the directory at d.basePath
func (d *DiskRecorder) Save(records record.MemoryRecords) (record.MemoryRecords, error) {
	d.lastRecording = records[0].At.UTC()
	name := fmt.Sprintf("insights-%s%s", d.lastRecording.Format("2006-01-02-150405"), archiveExtension)
	path := filepath.Join(d.basePath, name)

	return d.SaveAtPath(records, path)
}

// SaveAtPath the records into the archive at `path`
func (d *DiskRecorder) SaveAtPath(records record.MemoryRecords, path string) (record.MemoryRecords, error) {
	if !strings.HasSuffix(path, archiveExtension) {
		return nil, fmt.Errorf(`path should have suffix "%v"`, archiveExtension)
	}

	wrote := 0
	start := time.Now()
	defer func() {
		if wrote > 0 {
			klog.Infof("Wrote %d records to disk in %s", wrote, time.Since(start).Truncate(time.Millisecond))
		}
	}()

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o640)
	if err != nil {
		if os.IsExist(err) {
			klog.Errorf("Tried to copy to %s which already exists", path)
			return nil, err
		}
		return nil, fmt.Errorf("unable to create archive: %v", err)
	}
	defer f.Close()

	completed := make([]record.MemoryRecord, 0, len(records))
	defer func() {
		wrote = len(completed)
	}()

	klog.Infof("Writing %d records to %s", len(records), path)

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	for _, r := range records {
		if err := tw.WriteHeader(&tar.Header{
			Name:     r.Name,
			ModTime:  r.At,
			Mode:     int64(os.FileMode(0o640).Perm()),
			Size:     int64(len(r.Data)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			return nil, fmt.Errorf("unable to write tar header: %v", err)
		}
		if _, err := tw.Write(r.Data); err != nil {
			return nil, fmt.Errorf("unable to write tar entry: %v", err)
		}
		completed = append(completed, r)
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

// Prune the archives when there are more than count archives
func (d *DiskRecorder) PruneByCount(countTreshold int) error {
	files, err := os.ReadDir(d.basePath)
	if err != nil {
		return err
	}

	archiveCount := 0
	fileCreatedAt := time.Now()
	var lastArchive os.DirEntry
	for _, file := range files {
		fileInfo, err := file.Info()
		if err != nil {
			return err
		}

		if isNotArchiveFile(fileInfo) {
			klog.Infof("Skipping file %v", file.Name())
			continue
		}

		// Find the oldest archive
		if fileInfo.ModTime().Before(fileCreatedAt) {
			fileCreatedAt = fileInfo.ModTime()
			lastArchive = file
		}

		archiveCount++
	}

	if archiveCount <= countTreshold {
		klog.Infof("No archives to prune, current count is %d", archiveCount)
		return nil
	}

	// Removes the oldest file
	if err := os.Remove(filepath.Join(d.basePath, lastArchive.Name())); err != nil {
		return fmt.Errorf("failed to delete expired file: %v", err)
	}

	klog.Infof("Clean up deleted archive: %v", lastArchive.Name())
	return nil
}

// Prune the archives older than given time
func (d *DiskRecorder) Prune(olderThan time.Time) error {
	files, err := os.ReadDir(d.basePath)
	if err != nil {
		return err
	}

	count := 0
	var errors []string
	for _, file := range files {
		fileInfo, err := file.Info()
		if err != nil {
			continue
		}

		if isNotArchiveFile(fileInfo) {
			continue
		}

		if fileInfo.ModTime().After(olderThan) {
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
		klog.Infof("Deleted %d files older than %s", count, olderThan.UTC().Format(time.RFC3339))
	}

	return nil
}

// Summary implements summarizer interface to insights uploader
func (d *DiskRecorder) Summary(_ context.Context, since time.Time) (*insightsclient.Source, bool, error) {
	files, err := os.ReadDir(d.basePath)
	if err != nil {
		return nil, false, err
	}
	if len(files) == 0 {
		return nil, false, nil
	}
	recentFiles := make([]string, 0, len(files))

	var fileInfo fs.FileInfo
	for _, file := range files {
		fileInfo, err = file.Info()
		if err != nil {
			return nil, false, err
		}
		if isNotArchiveFile(fileInfo) {
			continue
		}
		if fileInfo.ModTime().Before(since) {
			continue
		}
		recentFiles = append(recentFiles, file.Name())
	}
	if len(recentFiles) == 0 {
		return nil, false, nil
	}
	lastFile := recentFiles[len(recentFiles)-1]
	klog.Infof("Found files to send: %v", lastFile)
	f, err := os.Open(filepath.Join(d.basePath, lastFile))
	if err != nil {
		return nil, false, nil
	}
	return &insightsclient.Source{Contents: f, CreationTime: d.lastRecording}, true, nil
}

func isNotArchiveFile(file os.FileInfo) bool {
	return file.IsDir() || !strings.HasPrefix(file.Name(), "insights-") || !strings.HasSuffix(file.Name(), ".tar.gz")
}

// LastArchive tries to find the latest Insights archive. Returns an error
// when it can't read the base directory or when it can't open the last archive found.
func (d *DiskRecorder) LastArchive() (*insightsclient.Source, error) {
	files, err := os.ReadDir(d.basePath)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}
	var lastTime time.Time
	var lastArchive string
	for _, file := range files {
		fileInfo, err := file.Info() // nolint: govet
		if err != nil {
			return nil, err
		}
		if isNotArchiveFile(fileInfo) {
			continue
		}
		if fileInfo.ModTime().After(lastTime) {
			lastTime = fileInfo.ModTime()
			lastArchive = file.Name()
		}
	}
	f, err := os.Open(filepath.Join(d.basePath, lastArchive))
	if err != nil {
		return nil, err
	}

	return &insightsclient.Source{Contents: f, CreationTime: d.lastRecording}, nil
}
