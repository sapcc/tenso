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
	"fmt"

	"github.com/sapcc/go-bits/logg"
	"github.com/sapcc/go-bits/sqlext"
)

var (
	gcDeliveredEventsQuery = sqlext.SimplifyWhitespace(`
		DELETE FROM events WHERE id NOT IN (SELECT event_id FROM pending_deliveries)
	`)
)

func (c *Context) CollectGarbage() (returnedError error) {
	defer func() {
		if returnedError == nil {
			gcSuccessCounter.Inc()
		} else {
			gcFailedCounter.Inc()
			returnedError = fmt.Errorf("while collecting garbage: %w", returnedError)
		}
	}()

	result, err := c.DB.Exec(gcDeliveredEventsQuery)
	if err != nil {
		return err
	}
	numDeleted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if numDeleted > 0 {
		logg.Info("cleaned up %d fully-delivered events", numDeleted)
	}

	return nil
}
