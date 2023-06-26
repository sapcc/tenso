# Tenso

Tenso is a microservice that is used within Converged Cloud to deliver and
translate application lifecycle events, most prominently regarding the
deployment of Helm releases.

The name comes from "tensō" (転送), which is Japanese for "data transfer".

## Data model

In this diagram, most database fields are elided for brevity.

```mermaid
erDiagram
    Event {
        string payload_type
        string payload
    }
    PendingDelivery {
        string payload_type
        string payload
    }
    KeystoneUser {
        string uuid
    }
    KeystoneUser ||--o{ Event : submits
    Event ||--|{ PendingDelivery : waits-on
```

Users can submit events via Tenso's own API. Once an event is submitted,
deliveries are scheduled according to the configured delivery routes. Since
the delivery of an event to a different system usually requires some sort of
data conversion, events and their deliveries each store their own payload and
the associated payload type information. For the event, the payload type is
provided by the user submitting the event to Tenso. For the delivery, the
target payload types are determined by Tenso's configuration.

## Usage

Build with `make`, install with `make install` or `docker build`. Run with
a single argument, either "api" or "worker", to select whether to expose the
HTTP API or run the background worker jobs. Configuration is provided via
environment variables.

### Configuration

The following environment variables are understood by both API and worker:

| Variable | Default | Explanation |
| -------- | ------- | ----------- |
| `TENSO_DB_NAME` | `tenso` | The name of the database. |
| `TENSO_DB_USERNAME` | `postgres` | Username of the database user. |
| `TENSO_DB_PASSWORD` | *(optional)* | Password for the specified user. |
| `TENSO_DB_HOSTNAME` | `localhost` | Hostname of the database server. |
| `TENSO_DB_PORT` | `5432` | Port on which the PostgreSQL service is running on. |
| `TENSO_DB_CONNECTION_OPTIONS` | *(optional)* | Database connection options. |
| `TENSO_ROUTES` | *(required)* | Comma-separated list of enabled delivery routes. Each route is a pair of payload types, separated by `->`. For example, `foo.v1 -> bar.v2` means that events with payload type `foo.v1` will be accepted by Tenso's API and then converted into payload type `bar.v2` for delivery. [See below](#supported-payload-types) for supported payload types. |

The following environment variables are only understood by the API:

| Variable | Default | Explanation |
| -------- | ------- | ----------- |
| `OS_...` | *(required)* | A full set of OpenStack auth environment variables for Tenso's service user. See [documentation for openstackclient][os-env] for details. |
| `TENSO_API_LISTEN_ADDRESS` | `:8080` | Listen address for HTTP server. |
| `TENSO_OSLO_POLICY_PATH` | *(required)* | Path to the `policy.[json|yaml]` file for this service. [See below](#api-specification) for details. |

The following environment variables are only understood by the worker:

| Variable | Default | Explanation |
| -------- | ------- | ----------- |
| `TENSO_WORKER_LISTEN_ADDRESS` | `:8080` | Listen address for HTTP server (only for healthcheck and Prometheus metrics). |

## API specification

Tenso has an OpenStack-style API, so a Keystone token must be supplied in the
`X-Auth-Token` header of all requests. The Tenso API can be discovered as
service type "tenso" in the Keystone catalog.

To control API access, Tenso understands access rules in the
[`oslo.policy` JSON][os-pol-json] and [`oslo.policy` YAML][os-pol-yaml]
formats.

See also: [List of available API attributes](https://github.com/sapcc/go-bits/blob/53eeb20fde03c3d0a35e76cf9c9a06b63a415e6b/gopherpolicy/pkg.go#L151-L164)

### `POST /v1/events/new`

Submits an event to Tenso for delivery. The event payload must be supplied in
the request body, in whatever format is appropriate for that payload type. On
success, 202 (Accepted) is returned.

| Query parameter | Explanation |
| --------------- | ----------- |
| `payload_type` | **Required.** The payload type for this event. [See below](#supported-payload-types) for supported payload types. |

The corresponding policy rule is `event:create`. The object attribute
`%(target.payload_type)s` can be used in this policy rule.

[os-env]: https://docs.openstack.org/python-openstackclient/latest/cli/man/openstack.html
[os-pol-json]: https://docs.openstack.org/oslo.policy/latest/admin/policy-json-file.html
[os-pol-yaml]: https://docs.openstack.org/oslo.policy/latest/admin/policy-yaml-file.html

### `POST /v1/events/synthetic`

Submits a synthetic event to Tenso for delivery. This endpoint works
identically to `POST /v1/events/new`, with two exceptions:

- Instead of reading the event payload in the request body, a hardcoded event
  payload with synthetic is used.
- The policy rule is `event:create_synthetic` instead of `event:create`.

This endpoint should be restricted to cloud admins. It can be used to test the
conversion and delivery path for an incoming payload type without having to
wait for an event to be submitted (or having to generate one manually).

## Supported payload types

### Helm deployments

When a Concourse pipeline performs a Helm deployment, we collect metadata and
status information about the deployment using our internal
concourse-release-resource. There are several payload types relating to this
event signaling process:

* `helm-deployment-from-concourse.v1` is supported on ingress and validates the
  event payload generated by the concourse-release-resource.
* `helm-deployment-to-elk.v1` forwards the payload from Concourse into Logstash
  for archival purposes.
* `helm-deployment-to-swift.v1` forwards the payload from Concourse into
  OpenStack Swift for archival purposes.
* `helm-deployment-to-servicenow.v1` forwards the payload from Concourse into
  the Change Management area of our ServiceNow instance.
* `infra-workflow-from-awx.v1` is supported on ingress and validates the event
  payload generated by our AWX workflows.
* `infra-workflow-to-swift.v1` forwards the payload from AWX into OpenStack
  Swift for archival purposes.
* `infra-workflow-to-servicenow.v1` forwards the payload from AWX into the
  Change Management area of our ServiceNow instance.

To configure these delivery paths, the following environment variables are inspected:

| Variable | Default | Explanation |
| -------- | ------- | ----------- |
| `TENSO_AWX_WORKFLOW_SWIFT_CONTAINER` | *(required)* | The name of the target Swift container for `infra-workflow-to-swift.v1` delivery. |
| `TENSO_HELM_DEPLOYMENT_LOGSTASH_HOST` | *(required)* | The host:port pair of the Logstash service for `helm-deployment-to-elk.v1` delivery. |
| `TENSO_HELM_DEPLOYMENT_SWIFT_CONTAINER` | *(required)* | The name of the target Swift container for `helm-deployment-to-swift.v1` delivery. |
| `TENSO_SERVICENOW_TOKEN_URL`<br>`TENSO_SERVICENOW_USERNAME`<br>`TENSO_SERVICENOW_PASSWORD` | *(required)* | OAuth2 credentials for authentication with the ServiceNow API. |
| `TENSO_SERVICENOW_CREATE_CHANGE_URL` | *(required)* | Endpoint URL for creating change objects in ServiceNow. |
| `TENSO_SERVICENOW_MAPPING_CONFIG_PATH` | *(required)* | Path to a config file containing additional configuration for the mapping between incoming events and ServiceNow change events. |

The config file for `TENSO_SERVICENOW_MAPPING_CONFIG_PATH` must be a YAML document with the following fields:

| Field | Data type | Explanation |
| ----- | --------- | ----------- |
| `helm-deployment` | object | Mapping ruleset for `helm-deployment-from-concourse.v1` translation to ServiceNow (see below). |
| `awx-workflow` | object | Mapping ruleset for `infra-workflow-from-awx.v1` translation to ServiceNow (see below). |
| `regions.<region>` | list of strings | Availability zones belonging to this region. |
| `availability_zones.<az>.datacenters` | list of strings | Data centers belonging to this AZ, using the names that ServiceNow expects. |
| `availability_zones.<az>.environment` | string | Either "Development", "QA" or "Production". |

The "mapping ruleset" objects mentioned above can have the following fields:

| Field | Data type | Explanation |
| ----- | --------- | ----------- |
| `change_model` | string | Value for the `chg_model` field. |
| `fallbacks.assignee` | string | User ID of the user that we will put into `assigned_to` when we don't have a better option. |
| `fallbacks.requester` | string | User ID of the user that we will put into `requested_by` when we don't have a better option. |
| `fallbacks.responsible_manager` | string | User name and ID of the user that we will put as `responsible_manager` when we don't have a better option. |
| `fallbacks.service_offering` | string | Value that we will put as `service_offering` when we don't have a better option. |
| `overrides.assignee` | string | If given, we will always put this into `assigned_to`. |
