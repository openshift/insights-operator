package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Support contains global configuration related to monitoring the health of the cluster
// for support-related purposes. It allows you to control whether health information (also
// referred to as cluster telemetry) is reported to Red Hat support systems.
type Support struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   SupportSpec   `json:"spec"`
	Status SupportStatus `json:"status"`
}

// SupportSpec contains user-modifiable fields that impact how support systems interact
// with this cluster.
type SupportSpec struct {
	// reportHealth indicates whether the cluster is permitted to send health and critical
	// configuration data to Red Hat support systems. This data is used to identify
	// whether upgrades are successful, assist in triaging hardware or software failures
	// in the core platform, and enable better support responses when failures do occur.
	// It also makes the cluster visible via the cloud.redhat.com console for central
	// insight into running clusters. The information reported to the support systems
	// does not contain information about workloads unless directly related to a core
	// subsystem malfunctioning.
	ReportHealth bool `json:"reportHealth"`
}

// SupportStatus will contain fields that the system chooses to report about how
// support is managed. It contains no fields now.
type SupportStatus struct {
}
