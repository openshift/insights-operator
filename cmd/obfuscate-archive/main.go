package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	installertypes "github.com/openshift/installer/pkg/types"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"

	"github.com/openshift/insights-operator/pkg/anonymization"
	"github.com/openshift/insights-operator/pkg/gather"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/recorder"
	"github.com/openshift/insights-operator/pkg/recorder/diskrecorder"
)

func main() {
	if len(os.Args) < 2 {
		_, _ = fmt.Fprintf(os.Stderr, "Path to the archive was not provided\n\n"+
			"Usage: go run ./cmd/obfuscate-archive/main.go PATH_TO_THE_ARCHIVE\n\n"+
			"Obfuscates the archive located at PATH_TO_THE_ARCHIVE\n")
		return
	}

	path := os.Args[1]

	if newPath, err := obfuscateArchive(path); err != nil {
		printlnToStderrf("Unable to obfuscate archive: %v", err)
	} else {
		fmt.Println("Created", newPath)
	}
}

func printlnToStderrf(format string, params ...interface{}) {
	_, _ = fmt.Fprintln(os.Stderr, fmt.Sprintf(format, params...))
}

func obfuscateArchive(path string) (string, error) {
	const suffix = ".tar.gz"
	if !strings.HasSuffix(path, suffix) {
		return "", fmt.Errorf(`invalid path to the archive: should end with "%v"`, suffix)
	}

	newPath := strings.TrimSuffix(path, suffix) + "-obfuscated" + suffix

	records, err := readArchive(path)
	if err != nil {
		return "", err
	}

	clusterBaseDomain, err := getClusterBaseDomain(records)
	if err != nil {
		return "", err
	}

	networks, err := anonymization.GetNetworksForAnonymizerFromRecords(records)
	if err != nil {
		return "", err
	}

	anonymizer, err := anonymization.NewAnonymizer(clusterBaseDomain, networks, nil)
	if err != nil {
		return "", err
	}

	var anonymizedRecords record.MemoryRecords

	for _, r := range records {
		if r.Name == recorder.MetadataRecordName+".json" {
			var metadata gather.ArchiveMetadata

			err = json.Unmarshal(r.Data, &metadata)
			if err != nil {
				return "", err
			}

			metadata.IsGlobalObfuscationEnabled = true

			metadataBytes, err := json.Marshal(metadata) //nolint:govet
			if err != nil {
				return "", err
			}

			r.Data = metadataBytes
		}

		anonymizedRecords = append(anonymizedRecords, *anonymizer.AnonymizeMemoryRecord(r))
	}

	diskRecorder := diskrecorder.New("")

	_, err = diskRecorder.SaveAtPath(anonymizedRecords, newPath)
	if err != nil {
		return "", err
	}

	return newPath, nil
}

func getClusterBaseDomain(records map[string]*record.MemoryRecord) (string, error) {
	domain, err := getClusterBaseDomainFromInfrastructureRecord(records)
	if err == nil {
		return domain, nil
	}

	printlnToStderrf(
		"Unable to get base domain from infrastructure record: %v. Trying to get it from install-config...",
		err,
	)

	return getClusterBaseDomainFromClusterConfigV1Record(records)
}

func getClusterBaseDomainFromInfrastructureRecord(records map[string]*record.MemoryRecord) (string, error) {
	const filePath = "config/infrastructure.json"

	r, found := records[filePath]
	if !found {
		return "", fmt.Errorf("%v record needed to fetch cluster base domain wasn't found", filePath)
	}

	var infrastructure configv1.Infrastructure

	err := json.Unmarshal(r.Data, &infrastructure)
	if err != nil {
		return "", err
	}

	domain := infrastructure.Status.EtcdDiscoveryDomain
	if len(domain) == 0 {
		return "", fmt.Errorf("status.etcdDiscoveryDomain from %v is empty", filePath)
	}

	return domain, nil
}

func getClusterBaseDomainFromClusterConfigV1Record(records map[string]*record.MemoryRecord) (string, error) {
	const filePath = "config/configmaps/kube-system/cluster-config-v1.json"

	r, found := records[filePath]
	if !found {
		return "", fmt.Errorf("%v record needed to fetch cluster base domain wasn't found", filePath)
	}

	var configMap corev1.ConfigMap

	err := json.Unmarshal(r.Data, &configMap)
	if err != nil {
		return "", err
	}

	installConfigStr, found := configMap.Data["install-config"]
	if !found {
		return "", fmt.Errorf("unable to find install-config")
	}

	var installConfig installertypes.InstallConfig

	err = yaml.Unmarshal([]byte(installConfigStr), &installConfig)
	if err != nil {
		return "", err
	}

	if len(installConfig.BaseDomain) == 0 {
		return "", fmt.Errorf("installConfig.BaseDomain from %v is empty", filePath)
	}

	return installConfig.BaseDomain, nil
}

func readArchive(path string) (map[string]*record.MemoryRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}

	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	records := make(map[string]*record.MemoryRecord)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		content, err := ioutil.ReadAll(tarReader)
		if err != nil {
			return nil, err
		}

		records[header.Name] = &record.MemoryRecord{
			Name: header.Name,
			Data: content,
		}
	}

	return records, nil
}
