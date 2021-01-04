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

package helpers

import "encoding/json"

// ToJSONString converts anything to JSON or panics if it's not possible
func ToJSONString(obj interface{}) string {
	return toJSONString(obj, false)
}

// ToJSONPrettyString converts anything to indented JSON or panics if it's not possible
func ToJSONPrettyString(obj interface{}) string {
	return toJSONString(obj, true)
}

// toJSONString converts anything to JSON or panics if it's not possible
// isOutputPretty makes output indented
func toJSONString(obj interface{}, isOutputPretty bool) string {
	var (
		jsonBytes []byte
		err       error
	)
	if isOutputPretty {
		jsonBytes, err = json.MarshalIndent(obj, "", "\t")
	} else {
		jsonBytes, err = json.Marshal(obj)
	}
	if err != nil {
		panic(err)
	}

	return string(jsonBytes)
}
