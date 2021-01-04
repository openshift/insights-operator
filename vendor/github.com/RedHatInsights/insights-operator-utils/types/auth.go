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
