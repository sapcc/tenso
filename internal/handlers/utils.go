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

package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/sapcc/go-api-declarations/deployevent"
)

func jsonUnmarshalStrict[T any](payload []byte) (T, error) {
	var data T
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	err := dec.Decode(&data)
	return data, err
}

////////////////////////////////////////////////////////////////////////////////
// shared functions for deployevent.Event (used by helm-deployment and
// terraform-deployment)

var (
	regionRx      = regexp.MustCompile(`^[a-z]{2}-[a-z]{2}-[0-9]$`)                                                // e.g. "qa-de-1"
	clusterRx     = regexp.MustCompile(`^(?:(?:|[a-z]-|ci[0-9]?-|gh-actions-)?[a-z]{2}-[a-z]{2}-[0-9]|k-master)$`) // e.g. "qa-de-1" or "s-qa-de-1" or "ci-eu-de-2" or "gh-actions-eu-de-2" or "k-master"
	gitCommitRx   = regexp.MustCompile(`^[0-9a-f]{40}$`)                                                           // SHA-1 digest with lower-case digits
	buildNumberRx = regexp.MustCompile(`^[1-9][0-9]*(?:\.[1-9][0-9]*)?$`)                                          // e.g. "23" or "42.1"
	sapUserIDRx   = regexp.MustCompile(`^(?:C[0-9]{7}|[DI][0-9]{6})$`)                                             // e.g. "D123456" or "C1234567"
)

func isClusterLocatedInRegion(cluster, region string) bool {
	if cluster == "k-master" {
		return region == "eu-nl-1"
	}
	return strings.HasSuffix(cluster, region)
}

func parseAndValidateDeployEvent(payload []byte) (deployevent.Event, error) {
	event, err := jsonUnmarshalStrict[deployevent.Event](payload)
	if err != nil {
		return deployevent.Event{}, err
	}

	if !regionRx.MatchString(event.Region) {
		return event, fmt.Errorf(`value for field region is invalid: %q`, event.Region)
	}
	if event.RecordedAt == nil {
		return event, errors.New("value for field recorded_at is missing")
	}

	for repoName, repoInfo := range event.GitRepos {
		if !gitCommitRx.MatchString(repoInfo.CommitID) {
			return event, fmt.Errorf(`value for field git[%q].commit-id is invalid: %q`, repoName, repoInfo.CommitID)
		}
	}

	//TODO: Can we validate values for TeamName by providing a set of valid values in env?
	if !buildNumberRx.MatchString(event.Pipeline.BuildNumber) {
		return event, fmt.Errorf("field pipeline.build-number is invalid: %q", event.Pipeline.BuildNumber)
	}
	_, err = url.Parse(event.Pipeline.BuildURL)
	if err != nil {
		return event, fmt.Errorf("field pipeline.build-url is invalid: %q", event.Pipeline.BuildURL)
	}
	if event.Pipeline.JobName == "" {
		return event, fmt.Errorf("field pipeline.job is invalid: %q", event.Pipeline.JobName)
	}
	if event.Pipeline.PipelineName == "" {
		return event, fmt.Errorf("field pipeline.name is invalid: %q", event.Pipeline.PipelineName)
	}
	if event.Pipeline.TeamName == "" {
		return event, fmt.Errorf("field pipeline.team is invalid: %q", event.Pipeline.TeamName)
	}
	if event.Pipeline.CreatedBy != "" && !sapUserIDRx.MatchString(event.Pipeline.CreatedBy) {
		return event, fmt.Errorf("field pipeline.created-by is invalid: %q", event.Pipeline.CreatedBy)
	}

	return event, nil
}

func inputDescriptorsOf(event deployevent.Event) (result []string) {
	var imageVersions []string
	for _, rel := range event.HelmReleases {
		if rel.ImageVersion != "" {
			imageVersions = append(imageVersions, fmt.Sprintf("%s %s", rel.Name, rel.ImageVersion))
		}
	}

	var gitVersions []string
	for name, repo := range event.GitRepos {
		// `name` is the name of this resource from which the Git repository was
		// pulled, which can be readable like `helm-charts.git` or `secrets.git`,
		// but sometimes is nonsensical without context (e.g. `qa-de-1.git` for a
		// checkout of `secrets.git` with path filter on qa-de-1 values), so we're
		// only using it if we don't have a better alternative

		if repo.RemoteURL != "" {
			// our preference is to take the basename from the remote URL, e.g.
			// remoteURL = "https://github.com/sapcc/helm-charts/"
			//   -> name = "helm-charts.git"
			remoteURL, err := url.Parse(repo.RemoteURL)
			if err == nil {
				name = strings.TrimSuffix(path.Base(strings.TrimSuffix(remoteURL.Path, "/")), ".git")
			}
		}
		if !strings.HasSuffix(name, ".git") {
			name += ".git"
		}

		gitVersions = append(gitVersions, fmt.Sprintf("%s %s", name, repo.CommitID))
	}
	sort.Strings(gitVersions) // for test reproducability

	return append(imageVersions, gitVersions...)
}
