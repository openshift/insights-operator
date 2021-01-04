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
	"testing"

	"github.com/stretchr/testify/assert"
)

// FailOnError logs error and stops next test's execution if non nil value is passed to err
// optionally, you can add a message
func FailOnError(t testing.TB, err error, msgAndArgs ...interface{}) {
	// assert.NoError is used to show human readable output
	assert.NoError(t, err, msgAndArgs...)
	// assert.NoError doesn't stop next test execution which can cause strange panic because
	// there was error and some object was not constructed
	if err != nil {
		t.Fatal(err)
	}
}
