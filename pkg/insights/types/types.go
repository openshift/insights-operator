package types

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// Timestamp represents any timestamp in a form gathered from database
type Timestamp string

// RuleID represents type for rule id
type RuleID string

// ErrorKey represents type for error key
type ErrorKey string

// UserVote is a type for user's vote
type UserVote int

// Response represents the Smart Proxy report response structure
type Response struct {
	Report SmartProxyReport `json:"report"`
}

// ReportResponseMeta contains metadata about the report
type ReportResponseMeta struct {
	Count         int       `json:"count"`
	LastCheckedAt Timestamp `json:"last_checked_at"`
	GatheredAt    Timestamp `json:"gathered_at"`
}

// RuleWithContentResponse represents a single rule in the response of /report endpoint
type RuleWithContentResponse struct {
	RuleID          RuleID      `json:"rule_id"`
	CreatedAt       string      `json:"created_at"`
	Description     string      `json:"description"`
	Generic         string      `json:"details"`
	Reason          string      `json:"reason"`
	Resolution      string      `json:"resolution"`
	MoreInfo        string      `json:"more_info"`
	TotalRisk       int         `json:"total_risk"`
	RiskOfChange    int         `json:"risk_of_change"`
	Disabled        bool        `json:"disabled"`
	DisableFeedback string      `json:"disable_feedback"`
	DisabledAt      Timestamp   `json:"disabled_at"`
	Internal        bool        `json:"internal"`
	UserVote        UserVote    `json:"user_vote"`
	TemplateData    interface{} `json:"extra_data"`
	Tags            []string    `json:"tags"`
}

// SmartProxyReport represents the response of /report endpoint for smart proxy
type SmartProxyReport struct {
	Meta ReportResponseMeta        `json:"meta"`
	Data []RuleWithContentResponse `json:"data"`
}

// InsightsRecommendation is a helper structure to store information about
// active Insights recommendations.
type InsightsRecommendation struct {
	RuleID RuleID
	// ErrorKey contains the original error_key value retrieved from
	// TemplateData rather than what the report contains at the highest level,
	// which is an ignored field that is unusable for any meaningful purpose.
	// Because of that, it is a string instead of the special ErrorKey type.
	ErrorKey    string
	Description string
	TotalRisk   int
}

type Recommendation struct {
	ErrorKey    string `json:"error_key"`
	Description string `json:"description"`
	TotalRisk   int    `json:"total_risk"`
	RuleFQDN    string `json:"rule_fqdn"`
}

// InsightsAnalysisReport is a type reflecting json structure
// of the insights-results-aggregator response. This is currently
// used only in TechPreview
type InsightsAnalysisReport struct {
	DownloadedAt    metav1.Time
	ClusterID       string           `json:"cluster"`
	Recommendations []Recommendation `json:"report"`
	RequestID       string           `json:"requestID"`
}
