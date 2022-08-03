/******************************************************************************
*
*  Copyright 2022 SAP SE
*
*  Licensed under the Apache License, Version 2.0 (the "License");
*  you may not use this file except in compliance with the License.
*  You may obtain a copy of the License at
*
*      http://www.apache.org/licenses/LICENSE-2.0
*
*  Unless required by applicable law or agreed to in writing, software
*  distributed under the License is distributed on an "AS IS" BASIS,
*  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
*  See the License for the specific language governing permissions and
*  limitations under the License.
*
******************************************************************************/

package test

import "testing"

// Must is a test assertion to test for success.
func Must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err.Error())
	}
}

// MustFail is a test assertion to test for a specific error message.
func MustFail(t *testing.T, actual error, expected string) {
	t.Helper()
	if actual == nil {
		t.Fatal("unexpected success, expected error: " + expected)
	}
	if actual.Error() != expected {
		t.Error("expected error: " + expected)
		t.Error(" but got error: " + actual.Error())
	}
}
