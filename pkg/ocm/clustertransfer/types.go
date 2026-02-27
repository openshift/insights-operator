package clustertransfer

import "time"

// pullSecretContent type representing pull-secret
type pullSecretContent struct {
	Auths       map[string]auth   `json:"auths"`
	HTTPHeaders map[string]string `json:"HttpHeaders,omitempty"`
}

// auth type representing "auth" item in the pull-secret
type auth struct {
	Auth  string `json:"auth"`
	Email string `json:"email,omitempty"`
}

type clusterTransferList struct {
	Page      int               `json:"page"`
	Total     int               `json:"total"`
	Size      int               `json:"size"`
	Transfers []clusterTransfer `json:"items"`
}

// clusterTransfer type represents the cluster transfer structure received from the OCM API
type clusterTransfer struct {
	ID             string    `json:"id,omitempty"`
	Href           string    `json:"href,omitempty"`
	ClusterUUID    string    `json:"cluster_uuid,omitempty"`
	Owner          string    `json:"owner,omitempty"`
	Recipient      string    `json:"recipient,omitempty"`
	ExpirationDate time.Time `json:"expiration_date,omitempty"`
	Status         string    `json:"status,omitempty"`
	Secret         string    `json:"secret,omitempty"` // nolint:gosec
	CreatedAt      time.Time `json:"created_at,omitempty"`
	UpdatedAt      time.Time `json:"updated_at,omitempty"`
}
