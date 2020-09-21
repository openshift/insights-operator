package insightsreport

// RuleID represents type for rule id
type RuleID string

// ErrorKey represents type for error key
type ErrorKey string

// UserVote is a type for user's vote
type UserVote int

// Timestamp represents any timestamp in a form gathered from database
type Timestamp string

// ReportResponseMeta contains metadata about the report
type ReportResponseMeta struct {
	Count         int       `json:"count"`
	LastCheckedAt Timestamp `json:"last_checked_at"`
}

// RuleWithContentResponse represents a single rule in the response of /report endpoint
type RuleWithContentResponse struct {
	RuleID       RuleID      `json:"rule_id"`
	ErrorKey     ErrorKey    `json:"-"`
	CreatedAt    string      `json:"created_at"`
	Description  string      `json:"description"`
	Generic      string      `json:"details"`
	Reason       string      `json:"reason"`
	Resolution   string      `json:"resolution"`
	TotalRisk    int         `json:"total_risk"`
	RiskOfChange int         `json:"risk_of_change"`
	Disabled     bool        `json:"disabled"`
	Internal     bool        `json:"internal"`
	UserVote     UserVote    `json:"user_vote"`
	TemplateData interface{} `json:"extra_data"`
	Tags         []string    `json:"tags"`
}

// SmartProxyReport represents the response of /report endpoint for smart proxy
type SmartProxyReport struct {
	Meta ReportResponseMeta        `json:"meta"`
	Data []RuleWithContentResponse `json:"data"`
}
