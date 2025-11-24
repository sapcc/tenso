<!--
SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company
SPDX-License-Identifier: Apache-2.0
-->

# `certs/`

If your Tenso build needs to accept additional (e.g. company-internal) CA certificates,
put them into this folder as a `.crt` file, and `docker build` will pick them up automatically.
