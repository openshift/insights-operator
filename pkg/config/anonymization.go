package config

import (
	"github.com/openshift/insights-operator/pkg/utils"
)

const (
	ClusterBaseDomainPlaceholder      = "<CLUSTER_BASE_DOMAIN>"
	ClusterBaseDomainAnonymizationKey = "clusterBaseDomain"
)

func (s *Serialized) fillAnonymizationConfig(cfg *Controller) {
	cfg.DisabledGlobalAnonymizations.DisableClusterBaseDomainAnonymization = utils.StringInSlice(
		ClusterBaseDomainAnonymizationKey, s.DisabledGlobalAnonymizations,
	)
}
