# Tenso

Tenso is a microservice that is used within Converged Cloud to deliver and
translate application lifecycle events, most prominently regarding the
deployment of Helm releases.

The name comes from "tensō" (転送), which is Japanese for "data transfer".

## Usage

Build with `make`, install with `make install` or `docker build`. Run without
a single argument, either "api" or "worker", to select whether to expose the
HTTP API or run the background worker jobs. The following environment variables
are understood by both API and worker:

| Variable | Default | Explanation |
| -------- | ------- | ----------- |
| `TENSO_DB_NAME` | `tenso` | The name of the database. |
| `TENSO_DB_USERNAME` | `postgres` | Username of the database user. |
| `TENSO_DB_PASSWORD` | *(optional)* | Password for the specified user. |
| `TENSO_DB_HOSTNAME` | `localhost` | Hostname of the database server. |
| `TENSO_DB_PORT` | `5432` | Port on which the PostgreSQL service is running on. |
| `TENSO_DB_CONNECTION_OPTIONS` | *(optional)* | Database connection options. |

TODO env vars for only API and for only worker
