// Package anonymization provides Anonymizer which is used to anonymize sensitive data. At the moment,
// anonymization is applied to all the data before storing it in the archive(see AnonymizeMemoryRecordFunction).
// There are the following global anonymizations which can be disabled in the config:
//   - clusterBaseDomain anonymizes cluster base domain.
//     For example, if the cluster base domain is `openshift.example.com`,
//     all the occurrences of this keyword will be replaced with `<CLUSTER_BASE_DOMAIN>`,
//     `cluster-api.openshift.example.com` will become `cluster-api.<CLUSTER_BASE_DOMAIN>`
package anonymization

import (
	"bytes"
	"strings"

	"github.com/openshift/insights-operator/pkg/config"
	"github.com/openshift/insights-operator/pkg/config/configobserver"
	"github.com/openshift/insights-operator/pkg/record"
)

// Anonymizer is used to anonymize sensitive data.
// Config can be used to disable anonymization of particular types of data.
type Anonymizer struct {
	configObserver    *configobserver.Controller
	clusterBaseDomain string
}

// NewAnonymizer creates a new instance of anonymizer with a provided config and sensitive data
func NewAnonymizer(configObserver *configobserver.Controller, clusterBaseDomain string) *Anonymizer {
	return &Anonymizer{
		configObserver:    configObserver,
		clusterBaseDomain: strings.TrimSpace(clusterBaseDomain),
	}
}

// AnonymizeMemoryRecord takes record.MemoryRecord, removes the sensitive data from it and returns the same object
func (anonymizer *Anonymizer) AnonymizeMemoryRecord(memoryRecord *record.MemoryRecord) *record.MemoryRecord {
	if anonymizer.configObserver == nil {
		return memoryRecord
	}

	conf := anonymizer.configObserver.Config().DisabledGlobalAnonymizations

	if !conf.DisableClusterBaseDomainAnonymization && len(anonymizer.clusterBaseDomain) != 0 {
		memoryRecord.Data = bytes.ReplaceAll(
			memoryRecord.Data,
			[]byte(anonymizer.clusterBaseDomain),
			[]byte(config.ClusterBaseDomainPlaceholder),
		)
		// TODO: anonymize memoryRecord.Name?
	}

	return memoryRecord
}
