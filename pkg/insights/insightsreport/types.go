package insightsreport

// Timestamp represents any timestamp in a form gathered from database
type Timestamp string

// ReportResponseMeta contains metadata about the report
type ReportResponseMeta struct {
	Count         int       `json:"count"`
	LastCheckedAt Timestamp `json:"last_checked_at"`
	GatheredAt    Timestamp `json:"gathered_at"`
}

// RuleWithContentResponse represents a single rule in the response of /report endpoint
type RuleWithContentResponse struct {
	TotalRisk int `json:"total_risk"`
}

// SmartProxyReport represents the response of /report endpoint for smart proxy
type SmartProxyReport struct {
	Meta ReportResponseMeta        `json:"meta"`
	Data []RuleWithContentResponse `json:"data"`
}
