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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/sapcc/go-bits/osext"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/sapcc/tenso/internal/tenso"
)

// Client can submit change payloads to ServiceNow.
type Client struct {
	EndpointURL string
	HTTPClient  *http.Client
}

// NewClientWithOAuth returns a http.Client that obtains OAuth2 tokens as required.
// Credentials are read from `${PREFIX}_{TOKEN_URL,USERNAME,PASSWORD}` env vars.
func NewClientWithOAuth(envPrefix string) (*Client, error) {
	endpointURL, err := osext.NeedGetenv(envPrefix + "_CREATE_CHANGE_URL")
	if err != nil {
		return nil, err
	}
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
	return &Client{
		EndpointURL: endpointURL,
		HTTPClient:  cfg.Client(context.Background()),
	}, nil
}

// DeliverChangePayload delivers a change payload to ServiceNow. This function
// has the same interface as DeliverPayload() in the tenso.DeliveryHandler
// interface.
func (c *Client) DeliverChangePayload(payload []byte) (*tenso.DeliveryLog, error) {
	req, err := http.NewRequest(http.MethodPost, c.EndpointURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("while preparing request for POST %s: %w", c.EndpointURL, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("during POST %s: %w", c.EndpointURL, err)
	}
	defer resp.Body.Close()

	//on success, make a best-effort attempt to retrieve the object ID from the
	//response...
	if resp.StatusCode < 400 { //nolint:usestdlibvars
		var respData struct {
			Result struct {
				Number struct {
					Value string `json:"value"`
				} `json:"number"`
			} `json:"result"`
		}
		err := json.NewDecoder(resp.Body).Decode(&respData)
		if err == nil && respData.Result.Number.Value != "" {
			return &tenso.DeliveryLog{
				Message: fmt.Sprintf("created %s in ServiceNow", respData.Result.Number.Value),
			}, nil
		}
		//...but failure to retrieve it is not an error, because we want
		//to avoid double delivery of the same payload if at all possible
		return nil, nil
	}

	//unexpected error -> log response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("while reading response body for failed POST %s: %w", c.EndpointURL, err)
	}
	return nil, fmt.Errorf("POST failed with status %d and response: %q", resp.StatusCode, string(bodyBytes))
}
