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

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/rs/zerolog/log"
	"net/http"

	"github.com/RedHatInsights/insights-operator-utils/responses"
)

// responseDataError is used as the error message when the responses functions return an error
const responseDataError = "Unexpected error during response data encoding"

// RouterMissingParamError missing parameter in request
type RouterMissingParamError struct {
	ParamName string
}

func (e *RouterMissingParamError) Error() string {
	return fmt.Sprintf("Missing required param from request: %v", e.ParamName)
}

// RouterParsingError parsing error, for example string when we expected integer
type RouterParsingError struct {
	ParamName  string
	ParamValue interface{}
	ErrString  string
}

func (e *RouterParsingError) Error() string {
	return fmt.Sprintf(
		"Error during parsing param '%v' with value '%v'. Error: '%v'",
		e.ParamName, e.ParamValue, e.ErrString,
	)
}

// UnauthorizedError means server can't authorize you, for example the token is missing or malformed
type UnauthorizedError struct {
	ErrString string
}

func (e *UnauthorizedError) Error() string {
	return e.ErrString
}

// ForbiddenError means you don't have permission to do a particular action,
// for example your account belongs to a different organization
type ForbiddenError struct {
	ErrString string
}

func (e *ForbiddenError) Error() string {
	return e.ErrString
}

// NoBodyError error meaning that client didn't provide body when it's required
type NoBodyError struct{}

func (*NoBodyError) Error() string {
	return "client didn't provide request body"
}

// HandleServerError handles separate server errors and sends appropriate responses
func HandleServerError(writer http.ResponseWriter, err error) {
	log.Error().Err(err).Msg("handleServerError()")

	var respErr error

	switch err := err.(type) {
	case *RouterMissingParamError, *RouterParsingError, *json.SyntaxError, *NoBodyError, *ValidationError:
		respErr = responses.SendBadRequest(writer, err.Error())
	case *json.UnmarshalTypeError:
		respErr = responses.SendBadRequest(writer, "bad type in json data")
	case *ItemNotFoundError:
		respErr = responses.SendNotFound(writer, err.Error())
	case *UnauthorizedError:
		respErr = responses.SendUnauthorized(writer, err.Error())
	case *ForbiddenError:
		respErr = responses.SendForbidden(writer, err.Error())
	default:
		respErr = responses.SendInternalServerError(writer, "Internal Server Error")
	}

	if respErr != nil {
		log.Error().Err(respErr).Msg(responseDataError)
	}
}

// ErrOldReport is an error returned if a more recent already
// exists on the storage while attempting to write a report for a cluster.
var ErrOldReport = errors.New("More recent report already exists in storage")

// ItemNotFoundError shows that item with id ItemID wasn't found in the storage
type ItemNotFoundError struct {
	ItemID interface{}
}

// Error returns error string
func (e *ItemNotFoundError) Error() string {
	return fmt.Sprintf("Item with ID %+v was not found in the storage", e.ItemID)
}
