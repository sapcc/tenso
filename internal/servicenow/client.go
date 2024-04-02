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
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/sapcc/tenso/internal/tenso"
)

// ClientSet is a set of Client objects.
//
// This type appears in type MappingConfiguration.
type ClientSet map[string]*Client

// Init validates the provided clientset and recurses into Client.Init().
func (cs ClientSet) Init() error {
	if _, exists := cs["default"]; !exists {
		return errors.New(`no "default" endpoint client declared`)
	}

	for clientName, client := range cs {
		err := client.Init()
		if err != nil {
			return fmt.Errorf("in initialization of endpoint client %q: %w", clientName, err)
		}
	}
	return nil
}

// DeliverChangePayload delivers a change payload to ServiceNow. This function
// has the same interface as DeliverPayload() in the tenso.DeliveryHandler
// interface.
func (cs ClientSet) DeliverChangePayload(ctx context.Context, payload []byte, routingInfo map[string]string) (*tenso.DeliveryLog, error) {
	clientName, exists := routingInfo["servicenow-target"]
	if !exists {
		clientName = "default"
	}
	client, exists := cs[clientName]
	if !exists {
		return nil, fmt.Errorf("unknown routing info: servicenow-target=%q", clientName)
	}
	return client.DeliverChangePayload(ctx, payload)
}

// Client can submit change payloads to ServiceNow.
//
// This type appears in type MappingConfiguration through type ClientSet.
type Client struct {
	EndpointURL    string       `yaml:"url"`
	ClientCertPath string       `yaml:"client_cert"`
	PrivateKeyPath string       `yaml:"private_key"`
	httpClient     *http.Client `yaml:"-"`
}

// Init validates the provided Client config and initializes the internal HTTP client.
func (c *Client) Init() error {
	// in unit tests, we are setting this dummy value to circumvent the
	// client-cert loading
	if c.EndpointURL == "http://www.example.com" {
		return nil
	}

	switch {
	case c.EndpointURL == "":
		return errors.New(`missing "url" attribute`)
	case c.ClientCertPath == "":
		return errors.New(`missing "client_cert" attribute`)
	case c.PrivateKeyPath == "":
		return errors.New(`missing "private_key" attribute`)
	}
	cert, err := tls.LoadX509KeyPair(c.ClientCertPath, c.PrivateKeyPath)
	if err != nil {
		return fmt.Errorf("cannot load client certificate: %w", err)
	}

	c.httpClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
				MinVersion:   tls.VersionTLS12,
			},
			Proxy: http.ProxyFromEnvironment,
		},
	}
	return nil
}

// DeliverChangePayload delivers a change payload to ServiceNow.
// It is usually called through ClientSet.DeliverChangePayload().
func (c *Client) DeliverChangePayload(ctx context.Context, payload []byte) (*tenso.DeliveryLog, error) {
	// if the TranslationHandler wants us to ignore this payload, skip the delivery
	if string(payload) == "skip" {
		return nil, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.EndpointURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("while preparing request for POST %s: %w", c.EndpointURL, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("during POST %s: %w", c.EndpointURL, err)
	}
	defer resp.Body.Close()

	// on success, make a best-effort attempt to retrieve the object ID from the
	// response...
	if resp.StatusCode < 400 {
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
		// ...but failure to retrieve it is not an error, because we want
		// to avoid double delivery of the same payload if at all possible
		return nil, nil
	}

	// unexpected error -> log response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("while reading response body for failed POST %s: %w", c.EndpointURL, err)
	}
	return nil, fmt.Errorf("POST failed with status %d and response: %q", resp.StatusCode, string(bodyBytes))
}
