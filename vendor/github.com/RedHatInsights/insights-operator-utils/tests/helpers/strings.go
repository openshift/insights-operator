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

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// AssertStringsAreEqualJSON checks whether strings represent the same JSON
// (whitespaces and order of elements doesn't matter)
// and asserts error otherwise
func AssertStringsAreEqualJSON(t testing.TB, expected, got string) {
	replacer := strings.NewReplacer("\n", "", "\t", "")

	expected = replacer.Replace(expected)
	got = replacer.Replace(got)

	var obj1, obj2 interface{}

	err := json.Unmarshal([]byte(expected), &obj1)
	if err != nil {
		err = fmt.Errorf(`expected is not JSON. value = "%v", err = "%v"`, expected, err)
	}
	FailOnError(t, err)

	err = json.Unmarshal([]byte(got), &obj2)
	if err != nil {
		err = fmt.Errorf(`got is not JSON. value = "%v", err = "%v"`, got, err)
	}
	FailOnError(t, err)

	assert.Equal(
		t,
		obj1,
		obj2,
		fmt.Sprintf(`%v
and
%v
should represent the same json`, expected, got),
	)
}

// JSONUnmarshalStrict unmarshales json and returns error if some field exist in data,
// but not in outObj
func JSONUnmarshalStrict(data []byte, outObj interface{}) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()

	return d.Decode(outObj)
}

// IsStringJSON check if the string is a JSON
func IsStringJSON(str string) bool {
	var devNull interface{}
	err := json.Unmarshal([]byte(str), &devNull)
	return err == nil
}
