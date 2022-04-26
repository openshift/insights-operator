package ocm

const (
	// OCMAPIFailureCountThreshold defines how many unsuccessful responses from the OCM API in a row is tolerated
	// before the operator is marked as Degraded
	OCMAPIFailureCountThreshold = 5
)
