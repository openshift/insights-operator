package types

// ContextKey is a type for user authentication token in request
type ContextKey string

const (
	// ContextKeyUser is a constant for user authentication token in request
	ContextKeyUser = ContextKey("user")
)

// Internal contains information about organization ID
type Internal struct {
	OrgID OrgID `json:"org_id,string"`
}

// Identity contains internal user info
type Identity struct {
	AccountNumber UserID   `json:"account_number"`
	Internal      Internal `json:"internal"`
}

// Token is x-rh-identity struct
type Token struct {
	Identity Identity `json:"identity"`
}

// JWTPayload is structure that contain data from parsed JWT token
type JWTPayload struct {
	AccountNumber UserID `json:"account_number"`
	OrgID         OrgID  `json:"org_id,string"`
}
