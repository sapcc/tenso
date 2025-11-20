// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package handlers_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/majewsky/gg/jsonmatch"
	"github.com/sapcc/go-bits/easypg"
	"github.com/sapcc/go-bits/must"
)

func TestMain(m *testing.M) {
	easypg.WithTestDB(m, func() int { return m.Run() })
}

func expectTranslatedPayload(t *testing.T, actual []byte, fixturePath string) {
	t.Helper()
	buf := must.ReturnT(os.ReadFile(fixturePath))(t)
	var expected jsonmatch.Object
	must.SucceedT(t, json.Unmarshal(buf, &expected))
	for _, diff := range expected.DiffAgainst(actual) {
		t.Errorf("in translated payload: %s", diff.String())
	}
}
