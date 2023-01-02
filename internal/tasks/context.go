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

	//dependency injection slots (usually filled by ApplyDefaults(), but filled
	//with doubles in tests)
	timeNow func() time.Time

	//When Blocker is not nil, tasks that support concurrent operation will
	//withhold operations until this channel is closed.
	Blocker <-chan struct{}
}

func NewContext(cfg tenso.Configuration, db *gorp.DbMap) *Context {
	return &Context{cfg, db, time.Now, nil}
}

// OverrideTimeNow is used by unit tests to inject a mock clock.
func (c *Context) OverrideTimeNow(now func() time.Time) *Context {
	c.timeNow = now
	return c
}

// JobPoller is a function, usually a member function of type Context, that can
// be called repeatedly to obtain Job instances.
//
// If there are no jobs to work on right now, sql.ErrNoRows shall be returned
// to signal to the caller to slow down the polling.
type JobPoller func() (Job, error)

// Job is a job that can be transferred to a worker goroutine to be executed
// there.
type Job interface {
	Execute() error
}

// ExecuteOne is used by unit tests to find and execute exactly one instance of
// the given type of Job. sql.ErrNoRows is returned when there are no jobs of
// that type waiting.
func ExecuteOne(p JobPoller) error {
	j, err := p()
	if err != nil {
		return err
	}
	return j.Execute()
}
