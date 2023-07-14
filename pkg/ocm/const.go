package ocm

const (
	// FailureCountThreshold defines how many unsuccessful responses from the OCM API in a row is tolerated
	// before the operator is marked as Degraded
	FailureCountThreshold = 5
)
