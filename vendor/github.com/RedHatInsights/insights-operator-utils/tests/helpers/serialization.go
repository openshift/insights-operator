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
	"encoding/gob"
	"io/ioutil"
	"testing"
)

// MustGobSerialize serializes an object using gob or panics
func MustGobSerialize(t testing.TB, obj interface{}) []byte {
	buf := new(bytes.Buffer)

	err := gob.NewEncoder(buf).Encode(obj)
	FailOnError(t, err)

	res, err := ioutil.ReadAll(buf)
	FailOnError(t, err)

	return res
}
