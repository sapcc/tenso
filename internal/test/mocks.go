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

package test

import (
	"net/http"
	"time"

	policy "github.com/databus23/goslo.policy"
	"github.com/sapcc/go-bits/gopherpolicy"
)

//Clock is a deterministic clock for unit tests. It starts at the Unix epoch
//and only advances when Clock.StepBy() is called.
type Clock struct {
	currentTime int64
}

//Now reads the clock.
func (c *Clock) Now() time.Time {
	return time.Unix(c.currentTime, 0).UTC()
}

//StepBy advances the clock by the given duration.
func (c *Clock) StepBy(d time.Duration) {
	c.currentTime += int64(d / time.Second)
}

//MockValidator implements the gopherpolicy.Enforcer and gopherpolicy.Validator
//interfaces.
type MockValidator struct {
	ForbiddenRules map[string]bool
}

func (mv *MockValidator) Allow(rule string) {
	if mv.ForbiddenRules == nil {
		mv.ForbiddenRules = make(map[string]bool)
	}
	mv.ForbiddenRules[rule] = false
}

func (mv *MockValidator) Forbid(rule string) {
	if mv.ForbiddenRules == nil {
		mv.ForbiddenRules = make(map[string]bool)
	}
	mv.ForbiddenRules[rule] = true
}

func (mv *MockValidator) CheckToken(r *http.Request) *gopherpolicy.Token {
	return &gopherpolicy.Token{
		Enforcer: mv,
		Context: policy.Context{
			Auth: map[string]string{
				"user_name":        "testusername",
				"user_id":          "testuserid",
				"user_domain_name": "testdomainname",
			},
			Request: map[string]string{},
		},
	}
}

func (mv *MockValidator) Enforce(rule string, ctx policy.Context) bool {
	return !mv.ForbiddenRules[rule]
}
