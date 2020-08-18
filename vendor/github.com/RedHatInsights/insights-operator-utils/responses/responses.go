// Utils for REST API

/*
Copyright © 2019, 2020 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package responses

import (
	"encoding/json"
	"net/http"
)

const (
	contentType = "Content-Type"
	appJSON     = "application/json; charset=utf-8"
)

// setDefaultContentType is a helper function to set the Content-Type header
func setDefaultContentType(w http.ResponseWriter) {
	w.Header().Set(contentType, appJSON)
}

// BuildResponse builds response for RestAPI request
func BuildResponse(status string) map[string]interface{} {
	return map[string]interface{}{"status": status}
}

// BuildOkResponse builds simple "ok" response
func BuildOkResponse() map[string]interface{} {
	return map[string]interface{}{"status": "ok"}
}

// BuildOkResponseWithData builds response with status "ok" and data
func BuildOkResponseWithData(dataName string, data interface{}) map[string]interface{} {
	resp := map[string]interface{}{"status": "ok"}
	resp[dataName] = data
	return resp
}

// Send sends HTTP response with a provided statusCode
// data can be either string or map[string]interface{}
// if data is string it will send response like this:
// {"status": data} which is helpful for explaining error to the client
//
// Returned error value is based on error returned from json.Encoder
func Send(statusCode int, w http.ResponseWriter, data interface{}) error {
	setDefaultContentType(w)
	w.WriteHeader(statusCode)
	if status, ok := data.(string); ok {
		return json.NewEncoder(w).Encode(BuildResponse(status))
	} else if rawData, ok := data.([]byte); ok {
		_, err := w.Write(rawData)
		return err
	}

	return json.NewEncoder(w).Encode(data)
}

// SendOK returns JSON response with status OK 200
func SendOK(w http.ResponseWriter, data map[string]interface{}) error {
	return Send(http.StatusOK, w, data)
}

// SendCreated returns response with status Created 201
func SendCreated(w http.ResponseWriter, data map[string]interface{}) error {
	return Send(http.StatusCreated, w, data)
}

// SendAccepted returns response with status Accepted 202
func SendAccepted(w http.ResponseWriter, data map[string]interface{}) error {
	return Send(http.StatusAccepted, w, data)
}

// SendBadRequest returns error response with status Bad Request 400
func SendBadRequest(w http.ResponseWriter, err string) error {
	return Send(http.StatusBadRequest, w, err)
}

// SendUnauthorized returns error response for unauthorized access with status Unauthorized 401
func SendUnauthorized(w http.ResponseWriter, data map[string]interface{}) error {
	return Send(http.StatusUnauthorized, w, data)
}

// SendForbidden returns response with status Forbidden 403
func SendForbidden(w http.ResponseWriter, err string) error {
	return Send(http.StatusForbidden, w, err)
}

// SendNotFound returns response with status Not Found 404
func SendNotFound(w http.ResponseWriter, err string) error {
	return Send(http.StatusNotFound, w, err)
}

// SendInternalServerError returns response with status Internal Server Error 500
func SendInternalServerError(w http.ResponseWriter, err string) error {
	return Send(http.StatusInternalServerError, w, err)
}

// SendServiceUnavailable returns response with status Service Unavailable 503
func SendServiceUnavailable(w http.ResponseWriter, err string) error {
	return Send(http.StatusServiceUnavailable, w, err)
}
