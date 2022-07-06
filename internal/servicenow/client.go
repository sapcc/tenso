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

package servicenow

import (
	"context"
	"net/http"

	"github.com/sapcc/go-bits/osext"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

//NewClientWithOAuth returns a http.Client that obtains OAuth2 tokens as required.
//Credentials are read from `${PREFIX}_{TOKEN_URL,USERNAME,PASSWORD}` env vars.
func NewClientWithOAuth(envPrefix string) (*http.Client, error) {
	tokenURL, err := osext.NeedGetenv(envPrefix + "_TOKEN_URL")
	if err != nil {
		return nil, err
	}
	username, err := osext.NeedGetenv(envPrefix + "_USERNAME")
	if err != nil {
		return nil, err
	}
	password, err := osext.NeedGetenv(envPrefix + "_PASSWORD")
	if err != nil {
		return nil, err
	}

	cfg := clientcredentials.Config{
		TokenURL:     tokenURL,
		ClientID:     username,
		ClientSecret: password,
		AuthStyle:    oauth2.AuthStyleInParams,
	}
	return cfg.Client(context.Background()), nil
}
