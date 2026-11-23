package util

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"path"
	"strings"
)

// Archive is a parsed insights-operator tar.gz archive.
type Archive struct {
	files map[string][]byte
}

// OpenArchive parses a gzip-compressed tar archive, as produced by the
// insights-operator recorder, from the given bytes.
func OpenArchive(data []byte) (*Archive, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to open gzip reader: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	files := make(map[string][]byte)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar entry: %w", err)
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		if strings.Contains(header.Name, "..") || path.IsAbs(header.Name) {
			return nil, fmt.Errorf("archive entry has unsafe path: %q", header.Name)
		}

		content, err := io.ReadAll(tarReader)
		if err != nil {
			return nil, fmt.Errorf("failed to read content of %q: %w", header.Name, err)
		}

		files[header.Name] = content
	}

	return &Archive{files: files}, nil
}

// Files returns the names of all regular files in the archive.
func (a *Archive) Files() []string {
	names := make([]string, 0, len(a.files))
	for name := range a.files {
		names = append(names, name)
	}
	return names
}

// FindFile returns the content of the archive entry with the exact given
// name, if present.
func (a *Archive) FindFile(name string) ([]byte, bool) {
	content, ok := a.files[name]
	return content, ok
}
