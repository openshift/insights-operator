package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/insights-operator/pkg/record"
)

func Test_getClusterBaseDomainFromInfrastructureRecord(t *testing.T) {
	tests := []struct {
		name                 string
		infrastructure       *configv1.Infrastructure
		includeRecord        bool
		expectedDomain       string
		expectError          bool
		errorContains        string
		emptyDiscoveryDomain bool
		invalidJSON          bool
	}{
		{
			name: "success - valid infrastructure record",
			infrastructure: &configv1.Infrastructure{
				Status: configv1.InfrastructureStatus{
					EtcdDiscoveryDomain: "test-cluster.example.com",
				},
			},
			includeRecord:  true,
			expectedDomain: "test-cluster.example.com",
			expectError:    false,
		},
		{
			name:           "error - infrastructure record not found",
			includeRecord:  false,
			expectError:    true,
			errorContains:  "config/infrastructure.json record needed to fetch cluster base domain wasn't found",
			expectedDomain: "",
		},
		{
			name: "error - empty etcdDiscoveryDomain",
			infrastructure: &configv1.Infrastructure{
				Status: configv1.InfrastructureStatus{
					EtcdDiscoveryDomain: "",
				},
			},
			includeRecord:        true,
			emptyDiscoveryDomain: true,
			expectError:          true,
			errorContains:        "status.etcdDiscoveryDomain from config/infrastructure.json is empty",
			expectedDomain:       "",
		},
		{
			name: "error - invalid JSON",
			infrastructure: &configv1.Infrastructure{
				Status: configv1.InfrastructureStatus{
					EtcdDiscoveryDomain: "test.example.com",
				},
			},
			includeRecord:  true,
			invalidJSON:    true,
			expectError:    true,
			errorContains:  "invalid character",
			expectedDomain: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			records := make(map[string]*record.MemoryRecord)

			if tt.includeRecord {
				var infraData []byte
				var err error

				if tt.invalidJSON {
					infraData = []byte(`{invalid json`)
				} else {
					infraData, err = json.Marshal(tt.infrastructure)
					assert.NoError(t, err)
				}

				records["config/infrastructure.json"] = &record.MemoryRecord{
					Name: "config/infrastructure.json",
					Data: infraData,
				}
			}

			domain, err := getClusterBaseDomainFromInfrastructureRecord(records)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedDomain, domain)
			}
		})
	}
}

func Test_getClusterBaseDomainFromIngressRecord(t *testing.T) {
	tests := []struct {
		name           string
		ingress        *configv1.Ingress
		includeRecord  bool
		expectedDomain string
		expectError    bool
		errorContains  string
		emptyDomain    bool
		invalidJSON    bool
	}{
		{
			name: "success - domain with apps prefix",
			ingress: &configv1.Ingress{
				Spec: configv1.IngressSpec{
					Domain: "apps.test-cluster.example.com",
				},
			},
			includeRecord:  true,
			expectedDomain: "test-cluster.example.com",
			expectError:    false,
		},
		{
			name:          "error - ingress record not found",
			includeRecord: false,
			expectError:   true,
			errorContains: "config/ingress.json record needed to fetch cluster base domain wasn't found",
		},
		{
			name: "error - empty domain",
			ingress: &configv1.Ingress{
				Spec: configv1.IngressSpec{
					Domain: "",
				},
			},
			includeRecord: true,
			emptyDomain:   true,
			expectError:   true,
			errorContains: "ingress.Spec.Domain from config/ingress.json is empty",
		},
		{
			name: "invalid JSON returns nil error",
			ingress: &configv1.Ingress{
				Spec: configv1.IngressSpec{
					Domain: "test.example.com",
				},
			},
			includeRecord:  true,
			invalidJSON:    true,
			expectError:    false,
			expectedDomain: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			records := make(map[string]*record.MemoryRecord)

			if tt.includeRecord {
				var ingressData []byte
				var err error

				if tt.invalidJSON {
					ingressData = []byte(`{invalid json`)
				} else {
					ingressData, err = json.Marshal(tt.ingress)
					assert.NoError(t, err)
				}

				records["config/ingress.json"] = &record.MemoryRecord{
					Name: "config/ingress.json",
					Data: ingressData,
				}
			}

			domain, err := getClusterBaseDomainFromIngressRecord(records)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedDomain, domain)
			}
		})
	}
}

func Test_getClusterBaseDomain(t *testing.T) {
	tests := []struct {
		name                  string
		includeInfrastructure bool
		includeIngress        bool
		infrastructureDomain  string
		ingressDomain         string
		expectedDomain        string
		expectError           bool
		errorContains         string
		invalidInfrastructure bool
		emptyInfrastructure   bool
		testFallbackToIngress bool
	}{
		{
			name:                  "success - infrastructure record available",
			includeInfrastructure: true,
			infrastructureDomain:  "cluster.example.com",
			expectedDomain:        "cluster.example.com",
			expectError:           false,
		},
		{
			name:                  "success - fallback to ingress when infrastructure missing",
			includeInfrastructure: false,
			includeIngress:        true,
			ingressDomain:         "apps.cluster.example.com",
			expectedDomain:        "cluster.example.com",
			expectError:           false,
			testFallbackToIngress: true,
		},
		{
			name:                  "error - both infrastructure and ingress missing",
			includeInfrastructure: false,
			includeIngress:        false,
			expectError:           true,
			errorContains:         "config/ingress.json record needed to fetch cluster base domain wasn't found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			records := make(map[string]*record.MemoryRecord)

			if tt.includeInfrastructure {
				infrastructure := &configv1.Infrastructure{
					Status: configv1.InfrastructureStatus{},
				}

				if !tt.emptyInfrastructure {
					infrastructure.Status.EtcdDiscoveryDomain = tt.infrastructureDomain
				}

				infraData, err := json.Marshal(infrastructure)
				assert.NoError(t, err)

				records["config/infrastructure.json"] = &record.MemoryRecord{
					Name: "config/infrastructure.json",
					Data: infraData,
				}
			}

			if tt.includeIngress {
				ingress := &configv1.Ingress{
					Spec: configv1.IngressSpec{
						Domain: tt.ingressDomain,
					},
				}

				ingressData, err := json.Marshal(ingress)
				assert.NoError(t, err)

				records["config/ingress.json"] = &record.MemoryRecord{
					Name: "config/ingress.json",
					Data: ingressData,
				}
			}

			domain, err := getClusterBaseDomain(records)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedDomain, domain)
			}
		})
	}
}

func Test_readArchive(t *testing.T) {
	// Create temporary archive file
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test-archive.tar.gz")

	archiveFiles := map[string]string{
		"config/infrastructure.json": `{"status":{"etcdDiscoveryDomain":"test.example.com"}}`,
		"config/ingress.json":        `{"spec":{"domain":"apps.test.example.com"}}`,
		"metadata.json":              `{"version":"1.0"}`,
	}

	err := createTestArchive(archivePath, archiveFiles)
	assert.NoError(t, err)

	// Test readArchive function
	records, err := readArchive(archivePath)

	assert.NoError(t, err)
	assert.Equal(t, 3, len(records))

	// Verify each record
	expectedRecords := []string{"config/infrastructure.json", "config/ingress.json", "metadata.json"}
	for _, name := range expectedRecords {
		record, exists := records[name]
		assert.True(t, exists, "Record %s should exist", name)
		assert.Equal(t, name, record.Name)
		assert.NotEmpty(t, record.Data)

		// Verify content matches
		expectedContent := archiveFiles[name]
		assert.Equal(t, expectedContent, string(record.Data))
	}
}

func Test_readArchive_InvalidFiles(t *testing.T) {
	tests := []struct {
		name          string
		setupFile     func(t *testing.T) string
		expectError   bool
		errorContains string
	}{
		{
			name: "error - file does not exist",
			setupFile: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return filepath.Join(tmpDir, "nonexistent.tar.gz")
			},
			expectError:   true,
			errorContains: "no such file or directory",
		},
		{
			name: "error - not a gzip file",
			setupFile: func(t *testing.T) string {
				tmpDir := t.TempDir()
				filePath := filepath.Join(tmpDir, "invalid.tar.gz")
				err := os.WriteFile(filePath, []byte("not a gzip file"), 0600)
				assert.NoError(t, err)
				return filePath
			},
			expectError:   true,
			errorContains: "gzip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := tt.setupFile(t)

			records, err := readArchive(filePath)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, records)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, records)
			}
		})
	}
}

// Helper function to create a test tar.gz archive
func createTestArchive(path string, files map[string]string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	for name, content := range files {
		header := &tar.Header{
			Name: name,
			Mode: 0600,
			Size: int64(len(content)),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		if _, err := tarWriter.Write([]byte(content)); err != nil {
			return err
		}
	}

	return nil
}

func Test_obfuscateArchive_InvalidPath(t *testing.T) {
	newPath, err := obfuscateArchive("archive.tar")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid path to the archive: should end with")
	assert.Empty(t, newPath)
}

func Test_obfuscateArchive_MissingRecords(t *testing.T) {
	t.Run("error when infrastructure and ingress records missing", func(t *testing.T) {
		tmpDir := t.TempDir()
		archivePath := filepath.Join(tmpDir, "incomplete.tar.gz")

		// Create archive without infrastructure or ingress records
		files := map[string]string{
			"some/other/file.json": `{"data":"value"}`,
		}

		err := createTestArchive(archivePath, files)
		assert.NoError(t, err)

		newPath, err := obfuscateArchive(archivePath)
		assert.Error(t, err)
		assert.Empty(t, newPath)
		assert.Contains(t, err.Error(), "record needed to fetch cluster base domain wasn't found")
	})
}
