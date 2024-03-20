/*******************************************************************************
*
* Copyright 2022 SAP SE
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You should have received a copy of the License along with this
* program. If not, you may obtain a copy of the License at
*
*     http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
*
*******************************************************************************/

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
