package types

// Timestamp represents any timestamp in a form gathered from database
type Timestamp string

// RuleID represents type for rule id
type RuleID string

// ErrorKey represents type for error key
type ErrorKey string

// ReportResponseMeta contains metadata about the report
type ReportResponseMeta struct {
	Count         int       `json:"count"`
	LastCheckedAt Timestamp `json:"last_checked_at"`
	GatheredAt    Timestamp `json:"gathered_at"`
}

// RuleWithContentResponse represents a single rule in the response of /report endpoint
type RuleWithContentResponse struct {
	RuleID      RuleID   `json:"rule_id"`
	ErrorKey    ErrorKey `json:"-"`
	CreatedAt   string   `json:"created_at"`
	Description string   `json:"description"`
	Generic     string   `json:"details"`
	Reason      string   `json:"reason"`
	Resolution  string   `json:"resolution"`
	MoreInfo    string   `json:"more_info"`
	TotalRisk   int      `json:"total_risk"`
	Disabled    bool     `json:"disabled"`
	Internal    bool     `json:"internal"`
}

// SmartProxyReport represents the response of /report endpoint for smart proxy
type SmartProxyReport struct {
	Meta ReportResponseMeta        `json:"meta"`
	Data []RuleWithContentResponse `json:"data"`
}

// InsightsRecommendation is a helper structure to store information about
// active Insights recommendations.
type InsightsRecommendation struct {
	RuleID      RuleID
	ErrorKey    ErrorKey
	Description string
	TotalRisk   int
}
