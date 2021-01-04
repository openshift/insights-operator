/*
Copyright © 2020 Red Hat, Inc.

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

package helpers

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"reflect"

	"github.com/verdverm/frisby"
)

// FrisbyExpectItemInArray returns checker function for frisby to check if item is in the array
// Example:
//
// frisby.Create("test creating organization").
// Get(apiURL + "/organization").
// Send().
// ExpectStatus(200).
// Expect(helpers.FrisbyExpectItemInArray("organizations", 55))
//
// will check if 55 is in organizations, like here
// `{"organizations": [1, 2, 3, 55], "status": "ok"}`
func FrisbyExpectItemInArray(fieldName string, expectedItem interface{}) frisby.ExpectFunc {
	return func(f *frisby.Frisby) (bool, string) {
		var resp map[string]interface{}

		err := unmarshalResponseBodyToJSON(f.Resp.Body, &resp)
		if err != nil {
			return false, err.Error()
		}

		jsonResp := ToJSONString(resp)

		if _, exist := resp[fieldName]; !exist {
			return false, fmt.Sprintf("field %v does not exist in response %v", fieldName, jsonResp)
		}

		array, ok := resp[fieldName].([]interface{})
		if !ok {
			return false, fmt.Sprintf("field %v is not an array in response %v", fieldName, jsonResp)
		}

		for _, actualItem := range array {
			if reflect.DeepEqual(fmt.Sprint(expectedItem), fmt.Sprint(actualItem)) {
				return true, ""
			}
		}

		return false, fmt.Sprintf(
			"Item %v was not found in array %v in response %v",
			expectedItem, array, string(jsonResp),
		)
	}
}

func unmarshalResponseBodyToJSON(respBody io.ReadCloser, obj interface{}) error {
	bodyBytes, err := ioutil.ReadAll(respBody)
	if err != nil {
		return err
	}

	// error should not happen there, but we need to make errcheck tool happy
	err = respBody.Close()
	if err != nil {
		return err
	}

	err = json.Unmarshal(bodyBytes, obj)
	if err != nil {
		return err
	}

	return nil
}
