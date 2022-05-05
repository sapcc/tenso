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

package main

import (
	"github.com/sapcc/go-bits/logg"

	"github.com/sapcc/tenso/internal/tenso"
)

func main() {
	cfg := tenso.ParseConfiguration()
	db, err := tenso.InitDB(cfg.DatabaseURL)
	must(err)
	_ = db

	//TODO: add subcommands "api" and "worker"
}

func must(err error) {
	if err != nil {
		logg.Fatal(err.Error())
	}
}
