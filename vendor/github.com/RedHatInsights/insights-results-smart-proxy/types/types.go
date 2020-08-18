// Copyright 2020 Red Hat, Inc
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package types contains all user-defined data types used in Smart Proxy REST
// API service. Some types are the same as in other services (especially in
// Insights Results Aggregator, Content Service) and thus they are imported
// (based on) the common package insights-operator-utils/types.
package types

import "github.com/RedHatInsights/insights-operator-utils/types"

// UserID represents type for user id
type UserID = types.UserID

// ReportResponseMeta contains metadata about the report
type ReportResponseMeta = types.ReportResponseMeta

// Timestamp represents any timestamp in a form gathered from database
type Timestamp = types.Timestamp

// RuleWithContentResponse represents a single rule in the response of /report endpoint
type RuleWithContentResponse struct {
	RuleID       types.RuleID   `json:"rule_id"`
	ErrorKey     types.ErrorKey `json:"-"`
	CreatedAt    string         `json:"created_at"`
	Description  string         `json:"description"`
	Generic      string         `json:"details"`
	Reason       string         `json:"reason"`
	Resolution   string         `json:"resolution"`
	TotalRisk    int            `json:"total_risk"`
	RiskOfChange int            `json:"risk_of_change"`
	Disabled     bool           `json:"disabled"`
	Internal     bool           `json:"internal"`
	UserVote     types.UserVote `json:"user_vote"`
	TemplateData interface{}    `json:"extra_data"`
	Tags         []string       `json:"tags"`
}

// SmartProxyReport represents the response of /report endpoint for smart proxy
type SmartProxyReport struct {
	Meta types.ReportResponseMeta  `json:"meta"`
	Data []RuleWithContentResponse `json:"data"`
}

// UserVote is a type for user's vote
type UserVote = types.UserVote

const (
	// UserVoteDislike shows user's dislike
	UserVoteDislike = types.UserVoteDislike

	// UserVoteNone shows no vote from user
	UserVoteNone = types.UserVoteNone

	// UserVoteLike shows user's like
	UserVoteLike = types.UserVoteLike
)
