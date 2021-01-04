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
	"testing"
	"time"
)

// TestFunctionPtr pointer to test function
type TestFunctionPtr = func(testing.TB)

// RunTestWithTimeout runs test with timeToRun timeout and fails if it wasn't in time
func RunTestWithTimeout(t testing.TB, test TestFunctionPtr, timeToRun time.Duration) {
	timeout := time.After(timeToRun)
	done := make(chan bool)

	go func() {
		test(t)
		done <- true
	}()

	select {
	case <-timeout:
		t.Fatal("test ran out of time")
	case <-done:
	}
}
