// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company
// SPDX-License-Identifier: Apache-2.0

package tasks

import (
	"time"

	"github.com/go-gorp/gorp/v3"

	"github.com/sapcc/tenso/internal/tenso"
)

// Context holds things used by the various task implementations in this
// package.
type Context struct {
	Config tenso.Configuration
	DB     *gorp.DbMap

	// dependency injection slots (usually filled by ApplyDefaults(), but filled
	// with doubles in tests)
	timeNow func() time.Time
}

func NewContext(cfg tenso.Configuration, db *gorp.DbMap) *Context {
	return &Context{cfg, db, time.Now}
}

// OverrideTimeNow is used by unit tests to inject a mock clock.
func (c *Context) OverrideTimeNow(now func() time.Time) *Context {
	c.timeNow = now
	return c
}
