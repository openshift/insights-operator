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

package httputils

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/RedHatInsights/insights-operator-utils/types"
)

// GetRouterParam retrieves parameter from URL like `/organization/{org_id}`
func GetRouterParam(request *http.Request, paramName string) (string, error) {
	value, found := mux.Vars(request)[paramName]
	if !found {
		return "", &types.RouterMissingParamError{ParamName: paramName}
	}

	return value, nil
}

// GetRouterPositiveIntParam retrieves parameter from URL like `/organization/{org_id}`
// and check it for being valid and positive integer, otherwise returns error
func GetRouterPositiveIntParam(request *http.Request, paramName string) (uint64, error) {
	value, err := GetRouterParam(request, paramName)
	if err != nil {
		return 0, err
	}

	uintValue, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, &types.RouterParsingError{
			ParamName:  paramName,
			ParamValue: value,
			ErrString:  "unsigned integer expected",
		}
	}

	if uintValue == 0 {
		return 0, &types.RouterParsingError{
			ParamName:  paramName,
			ParamValue: value,
			ErrString:  "positive value expected",
		}
	}

	return uintValue, nil
}

// ReadClusterName retrieves cluster name from request
// if it's not possible, it writes http error to the writer and returns false
func ReadClusterName(writer http.ResponseWriter, request *http.Request) (types.ClusterName, bool) {
	clusterName, err := GetRouterParam(request, "cluster")
	if err != nil {
		handleClusterNameError(writer, err)
		return "", false
	}

	validatedClusterName, err := ValidateClusterName(clusterName)
	if err != nil {
		handleClusterNameError(writer, err)
		return "", false
	}

	return validatedClusterName, true
}

// ReadRuleID retrieves rule id from request's url or writes an error to writer.
// The function returns a rule id and a bool indicating if it was successful.
func ReadRuleID(writer http.ResponseWriter, request *http.Request) (types.RuleID, bool) {
	ruleID, err := GetRouterParam(request, "rule_id")
	if err != nil {
		const message = "unable to get rule id"
		log.Error().Err(err).Msg(message)
		types.HandleServerError(writer, err)
		return types.RuleID(0), false
	}

	ruleIDValidator := regexp.MustCompile(`^[a-zA-Z_0-9.]+$`)

	isRuleIDValid := ruleIDValidator.Match([]byte(ruleID))

	if !isRuleIDValid {
		err = fmt.Errorf("invalid rule ID, it must contain only from latin characters, number, underscores or dots")
		log.Error().Err(err)
		types.HandleServerError(writer, &types.RouterParsingError{
			ParamName:  "rule_id",
			ParamValue: ruleID,
			ErrString:  err.Error(),
		})
		return types.RuleID(0), false
	}

	return types.RuleID(ruleID), true
}

// ReadErrorKey retrieves error key from request's url or writes an error to writer.
// The function returns an error key and a bool indicating if it was successful.
func ReadErrorKey(writer http.ResponseWriter, request *http.Request) (types.ErrorKey, bool) {
	errorKey, err := GetRouterParam(request, "error_key")
	if err != nil {
		const message = "unable to get error_key"
		log.Error().Err(err).Msg(message)
		types.HandleServerError(writer, err)
		return types.ErrorKey(0), false
	}

	return types.ErrorKey(errorKey), true
}

// ReadOrganizationID retrieves organization id from request
// if it's not possible, it writes http error to the writer and returns false
func ReadOrganizationID(writer http.ResponseWriter, request *http.Request, auth bool) (types.OrgID, bool) {
	organizationID, err := GetRouterPositiveIntParam(request, "organization")
	if err != nil {
		HandleOrgIDError(writer, err)
		return 0, false
	}

	successful := CheckPermissions(writer, request, types.OrgID(organizationID), auth)

	return types.OrgID(organizationID), successful
}

// ReadClusterNames does the same as `readClusterName`, except for multiple clusters.
func ReadClusterNames(writer http.ResponseWriter, request *http.Request) ([]types.ClusterName, bool) {
	clusterNamesParam, err := GetRouterParam(request, "clusters")
	if err != nil {
		message := fmt.Sprintf("Cluster names are not provided %v", err.Error())
		log.Error().Msg(message)

		types.HandleServerError(writer, err)

		return []types.ClusterName{}, false
	}

	clusterNamesConverted := make([]types.ClusterName, 0)
	for _, clusterName := range SplitRequestParamArray(clusterNamesParam) {
		convertedName, err := ValidateClusterName(clusterName)
		if err != nil {
			types.HandleServerError(writer, err)
			return []types.ClusterName{}, false
		}

		clusterNamesConverted = append(clusterNamesConverted, convertedName)
	}

	return clusterNamesConverted, true
}

// ReadOrganizationIDs does the same as `readOrganizationID`, except for multiple organizations.
func ReadOrganizationIDs(writer http.ResponseWriter, request *http.Request) ([]types.OrgID, bool) {
	organizationsParam, err := GetRouterParam(request, "organizations")
	if err != nil {
		HandleOrgIDError(writer, err)
		return []types.OrgID{}, false
	}

	organizationsConverted := make([]types.OrgID, 0)
	for _, orgStr := range SplitRequestParamArray(organizationsParam) {
		orgInt, err := strconv.ParseUint(orgStr, 10, 64)
		if err != nil {
			types.HandleServerError(writer, &types.RouterParsingError{
				ParamName:  "organizations",
				ParamValue: orgStr,
				ErrString:  "integer array expected",
			})
			return []types.OrgID{}, false
		}
		organizationsConverted = append(organizationsConverted, types.OrgID(orgInt))
	}

	return organizationsConverted, true
}

// HandleOrgIDError logs org id error and writes corresponding http response
func HandleOrgIDError(writer http.ResponseWriter, err error) {
	log.Error().Err(err).Msg("error getting organization ID from request")
	types.HandleServerError(writer, err)
}

// CheckPermissions checks whether user with a provided token(from request) can access current organization
// and handled the error on negative result by logging the error and writing a corresponding http response
func CheckPermissions(writer http.ResponseWriter, request *http.Request, orgID types.OrgID, auth bool) bool {
	identityContext := request.Context().Value(types.ContextKeyUser)

	if identityContext != nil && auth {
		identity := identityContext.(types.Identity)
		if identity.Internal.OrgID != orgID {
			const message = "you have no permissions to get or change info about this organization"
			log.Error().Msg(message)
			types.HandleServerError(writer, &types.ForbiddenError{ErrString: message})

			return false
		}
	}
	return true
}

// ValidateClusterName checks that the cluster name is a valid UUID.
// Converted cluster name is returned if everything is okay, otherwise an error is returned.
func ValidateClusterName(clusterName string) (types.ClusterName, error) {
	if _, err := uuid.Parse(clusterName); err != nil {
		message := fmt.Sprintf("invalid cluster name: '%s'. Error: %s", clusterName, err.Error())

		log.Error().Err(err).Msg(message)

		return "", &types.RouterParsingError{
			ParamName:  "cluster",
			ParamValue: clusterName,
			ErrString:  err.Error(),
		}
	}

	return types.ClusterName(clusterName), nil
}

func handleClusterNameError(writer http.ResponseWriter, err error) {
	log.Error().Msg(err.Error())

	// query parameter 'cluster' can't be found in request, which might be caused by issue in Gorilla mux
	// (not on client side), but let's assume it won't :)
	types.HandleServerError(writer, err)
}

// SplitRequestParamArray takes a single HTTP request parameter and splits it
// into a slice of strings. This assumes that the parameter is a comma-separated array.
func SplitRequestParamArray(arrayParam string) []string {
	return strings.Split(arrayParam, ",")
}

// ReadClusterListFromPath retrieves list of clusters from request's path
// if it's not possible, it writes http error to the writer and returns false
func ReadClusterListFromPath(writer http.ResponseWriter, request *http.Request) ([]string, bool) {
	rawClusterList, err := GetRouterParam(request, "cluster_list")
	if err != nil {
		types.HandleServerError(writer, err)
		return []string{}, false
	}

	// basic check that should not happen in reality (because of Gorilla mux checks)
	if rawClusterList == "" {
		types.HandleServerError(writer, errors.New("cluster list is empty"))
		return []string{}, false
	}

	// split the list into items
	clusterList := strings.Split(rawClusterList, ",")

	// everything seems ok -> return list of clusters
	return clusterList, true
}

// ReadClusterListFromBody retrieves list of clusters from request's body
// if it's not possible, it writes http error to the writer and returns false
func ReadClusterListFromBody(writer http.ResponseWriter, request *http.Request) ([]string, bool) {
	var clusterList types.ClusterListInRequest

	// check if there's any body provided in the request sent by client
	if request.ContentLength <= 0 {
		err := &types.NoBodyError{}
		types.HandleServerError(writer, err)
		return []string{}, false
	}

	// try to read cluster list from request parameter
	err := json.NewDecoder(request.Body).Decode(&clusterList)
	if err != nil {
		types.HandleServerError(writer, err)
		return []string{}, false
	}

	// everything seems ok -> return list of clusters
	return clusterList.Clusters, true
}
