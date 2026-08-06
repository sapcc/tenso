// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package tasks_test

import (
	"testing"

	"go.xyrillian.de/gg/pgruntime"
)

func TestMain(m *testing.M) {
	pgruntime.WithTestDB(m, m.Run)
}
