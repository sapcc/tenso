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

package tenso

import (
	"fmt"
	"net"
	"net/url"
	"os"

	"github.com/sapcc/go-bits/logg"
)

//Configuration contains all configuration values that we collect from the environment.
type Configuration struct {
	DatabaseURL url.URL
}

//ParseConfiguration obtains a tenso.Configuration instance from the
//corresponding environment variables. Aborts on error.
func ParseConfiguration() Configuration {
	return Configuration{
		DatabaseURL: getDBURL(),
	}
}

//GetenvOrDefault is like os.Getenv but it also takes a default value which is
//returned if the given environment variable is missing or empty.
func GetenvOrDefault(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		val = defaultVal
	}
	return val
}

func getDBURL() url.URL {
	dbName := GetenvOrDefault("TENSO_DB_NAME", "tenso")
	dbUsername := GetenvOrDefault("TENSO_DB_USERNAME", "postgres")
	dbPass := os.Getenv("TENSO_DB_PASSWORD")
	dbHost := GetenvOrDefault("TENSO_DB_HOSTNAME", "localhost")
	dbPort := GetenvOrDefault("TENSO_DB_PORT", "5432")

	dbConnOpts, err := url.ParseQuery(os.Getenv("TENSO_DB_CONNECTION_OPTIONS"))
	if err != nil {
		logg.Fatal("while parsing TENSO_DB_CONNECTION_OPTIONS: " + err.Error())
	}
	hostname, err := os.Hostname()
	if err == nil {
		dbConnOpts.Set("application_name", fmt.Sprintf("tenso@%s", hostname))
	} else {
		dbConnOpts.Set("application_name", "tenso")
	}

	return url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(dbUsername, dbPass),
		Host:     net.JoinHostPort(dbHost, dbPort),
		Path:     dbName,
		RawQuery: dbConnOpts.Encode(),
	}
}
