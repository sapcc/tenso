// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

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
