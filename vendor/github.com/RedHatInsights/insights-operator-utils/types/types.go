// Package types contains declaration of various data types (usually structures)
// used elsewhere in the aggregator code.
package types

import (
	"time"
)

// OrgID represents organization ID
type OrgID uint32

// ClusterName represents name of cluster in format c8590f31-e97e-4b85-b506-c45ce1911a12
type ClusterName string

// UserID represents type for user id
type UserID string

// ClusterReport represents cluster report
type ClusterReport string

// Timestamp represents any timestamp in a form gathered from database
// TODO: need to be improved
type Timestamp string

// UserVote is a type for user's vote
type UserVote int

// RequestID is used to store the request ID supplied in input Kafka records as
// a unique identifier of payloads. Empty string represents a missing request ID.
type RequestID string

// RuleID represents type for rule id
type RuleID string

// RuleOnReport represents a single (hit) rule of the string encoded report
type RuleOnReport struct {
	Module       RuleID      `json:"component"`
	ErrorKey     ErrorKey    `json:"key"`
	UserVote     UserVote    `json:"user_vote"`
	Disabled     bool        `json:"disabled"`
	TemplateData interface{} `json:"details"`
}

// ReportRules is a helper struct for easy JSON unmarshalling of string encoded report
type ReportRules struct {
	HitRules     []RuleOnReport `json:"reports"`
	SkippedRules []RuleOnReport `json:"skips"`
	PassedRules  []RuleOnReport `json:"pass"`
	TotalCount   int
}

// ReportResponse represents the response of /report endpoint
type ReportResponse struct {
	Meta   ReportResponseMeta `json:"meta"`
	Report []RuleOnReport     `json:"reports"`
}

// ReportResponseMeta contains metadata about the report
type ReportResponseMeta struct {
	Count         int       `json:"count"`
	LastCheckedAt Timestamp `json:"last_checked_at"`
}

// RuleContentResponse represents a single rule in the response of /report endpoint
type RuleContentResponse struct {
	CreatedAt    string      `json:"created_at"`
	Description  string      `json:"description"`
	ErrorKey     string      `json:"-"`
	Generic      string      `json:"details"`
	Reason       string      `json:"reason"`
	Resolution   string      `json:"resolution"`
	TotalRisk    int         `json:"total_risk"`
	RiskOfChange int         `json:"risk_of_change"`
	RuleModule   RuleID      `json:"rule_id"`
	TemplateData interface{} `json:"extra_data"`
	Tags         []string    `json:"tags"`
	UserVote     UserVote    `json:"user_vote"`
	Disabled     bool        `json:"disabled"`
	Internal     bool        `json:"internal"`
}

// DisabledRuleResponse represents a single disabled rule displaying only identifying information
type DisabledRuleResponse struct {
	RuleModule  string `json:"rule_id"`
	Description string `json:"description"`
	Generic     string `json:"details"`
	DisabledAt  string `json:"disabled_at"`
}

// ErrorKey represents type for error key
type ErrorKey string

// Rule represents the content of rule table
type Rule struct {
	Module     RuleID `json:"module"`
	Name       string `json:"name"`
	Summary    string `json:"summary"`
	Reason     string `json:"reason"`
	Resolution string `json:"resolution"`
	MoreInfo   string `json:"more_info"`
}

// RuleErrorKey represents the content of rule_error_key table
type RuleErrorKey struct {
	ErrorKey    ErrorKey  `json:"error_key"`
	RuleModule  RuleID    `json:"rule_module"`
	Condition   string    `json:"condition"`
	Description string    `json:"description"`
	Impact      int       `json:"impact"`
	Likelihood  int       `json:"likelihood"`
	PublishDate time.Time `json:"publish_date"`
	Active      bool      `json:"active"`
	Generic     string    `json:"generic"`
	Tags        []string  `json:"tags"`
}

// RuleWithContent represents a rule with content, basically the mix of rule and rule_error_key tables' content
type RuleWithContent struct {
	Module       RuleID    `json:"module"`
	Name         string    `json:"name"`
	Summary      string    `json:"summary"`
	Reason       string    `json:"reason"`
	Resolution   string    `json:"resolution"`
	MoreInfo     string    `json:"more_info"`
	ErrorKey     ErrorKey  `json:"error_key"`
	Condition    string    `json:"condition"`
	Description  string    `json:"description"`
	TotalRisk    int       `json:"total_risk"`
	RiskOfChange int       `json:"risk_of_change"`
	PublishDate  time.Time `json:"publish_date"`
	Active       bool      `json:"active"`
	Internal     bool      `json:"internal"`
	Generic      string    `json:"generic"`
	Tags         []string  `json:"tags"`
}

// KafkaOffset type for kafka offset
type KafkaOffset int64

// DBDriver type for db driver enum
type DBDriver int

const (
	// DBDriverSQLite3 shows that db driver is sqlite
	DBDriverSQLite3 DBDriver = iota
	// DBDriverPostgres shows that db driver is postgres
	DBDriverPostgres
	// DBDriverGeneral general sql(used for mock now)
	DBDriverGeneral
)

const (
	// UserVoteDislike shows user's dislike
	UserVoteDislike UserVote = -1
	// UserVoteNone shows no vote from user
	UserVoteNone UserVote = 0
	// UserVoteLike shows user's like
	UserVoteLike UserVote = 1
)
