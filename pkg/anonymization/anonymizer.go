// Package anonymization provides Anonymizer which is used to anonymize sensitive data. At the moment,
// anonymization is applied to all the data before storing it in the archive(see AnonymizeMemoryRecordFunction).
// If you want to enable the anonymization you need to set "enableGlobalObfuscation" to "true" in config
// or "support" secret in "openshift-config" namespace, the anonymizer object then will be created and used
// (see pkg/controller/operator.go and pkg/controller/gather_job.go).
// When enabled, the following data will be anonymized:
//   - cluster base domain. For example, if the cluster base domain is `openshift.example.com`,
//     all the occurrences of this keyword will be replaced with `<CLUSTER_BASE_DOMAIN>`,
//     `cluster-api.openshift.example.com` will become `cluster-api.<CLUSTER_BASE_DOMAIN>`
//   - IPv4 addresses. Using a config client, it retrieves cluster networks and uses them to anonymize IP addresses
//     preserving subnet information. For example, if you have the following networks in your cluster:
//     "10.128.0.0/14", "172.30.0.0/16", "127.0.0.0/8"(added by default) the anonymization will handle the IPs like this:
//   - 10.128.0.0 -> 10.128.0.0  // subnetwork itself won't be anonymized
//   - 10.128.0.55 -> 10.128.0.1
//   - 10.128.0.56 -> 10.128.0.2
//   - 10.128.0.55 -> 10.128.0.1
//     // anonymizer maintains a translation table to replace the same original IPs with the same obfuscated IPs
//   - 10.129.0.0 -> 10.128.0.3
//   - 172.30.0.5 -> 172.30.0.1  // new subnet, so we use a new set of fake IPs
//   - 127.0.0.1 -> 127.0.0.1  // it was the first IP, so the new IP matched the original in this case
//   - 10.0.134.130 -> 0.0.0.0  // ip doesn't match any subnet, we replace such IPs with 0.0.0.0
package anonymization

import (
	"slices"

	"github.com/openshift/insights-operator/pkg/record"
)

type AnonymizerType string

const (
	NetworkAnonymizerType AnonymizerType = "networking"
)

type DataAnonymizer interface {
	// AnonymizeData processes the given memory record and returns anonymized version.
	AnonymizeData(memoryRecord *record.MemoryRecord) (*record.MemoryRecord, error)
	// IsEnabled returns if anonymizer is enabled and should be applied.
	IsEnabled() bool
	// GetType returns the type of the anonymizer implementation.
	GetType() AnonymizerType
}

// Anonymizer is used to anonymize sensitive data.
// Config can be used to enable anonymization of cluster base domain
// and obfuscation of IPv4 addresses
type Anonymizer struct {
	Anonymizers []DataAnonymizer
}

func NewAnonymizer(specificAnonymizer ...DataAnonymizer) (*Anonymizer, error) {
	return &Anonymizer{
		Anonymizers: specificAnonymizer,
	}, nil
}

func (anonymizer *Anonymizer) AnonymizeData(memoryRecord *record.MemoryRecord) (*record.MemoryRecord, error) {
	if memoryRecord == nil {
		return nil, nil
	}
	var err error
	anonymizedResult := memoryRecord

	for _, specificAnonymizer := range anonymizer.Anonymizers {
		if specificAnonymizer.IsEnabled() {
			anonymizedResult, err = specificAnonymizer.AnonymizeData(memoryRecord)
			if err != nil {
				return nil, err
			}
		}
	}

	return anonymizedResult, nil
}

func (anonymizer *Anonymizer) IsAnonymizerTypeEnabled(anonymizerType AnonymizerType) bool {
	return slices.ContainsFunc(anonymizer.Anonymizers, func(an DataAnonymizer) bool {
		return an.GetType() == anonymizerType && an.IsEnabled()
	})
}
