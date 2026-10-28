# terraform-provider-aiostreams

OpenTofu provider for one complete AIOStreams `UserData` document. The
provider targets the deployed AIOStreams `v2.37.0` contract and uses the
native `POST/GET?raw=true/PUT /api/v1/user` API. `PUT` is a full replacement;
the provider reads the raw document, overlays only its managed HCL/JSON fields,
and preserves unknown fields.

The `aiostreams_configuration` resource provides family profiles, independent
persona history, native preset/addon configuration, services, catalog
modifications, templates, language preferences and JSON overlays for the
remaining non-secret UserData fields. Secret values use OpenTofu write-only
attributes (`password_wo`, service credentials, profile PINs and secret overlays). Refreshes
use the process-only `AIOSTREAMS_PASSWORD` environment variable because
OpenTofu deliberately does not make write-only values available to `Read`.

Profile PINs use `lock_wo` plus the non-secret `lock_wo_version`. The write-only
PIN is sent only on creation or when its version changes; otherwise the remote
bcrypt hash is preserved. This allows a native AIOStreams/admin PIN change to
survive later Terraform reconciliations. AIOStreams v2.37.0 does not expose a
self-service persona PIN-change endpoint; changing a PIN remains a native
configuration edit, not a provider-specific UI or API.

The resource never calls `DELETE`; removing it from configuration returns a
diagnostic and leaves the remote account intact. The state contains the UUID,
non-secret configuration inputs and a redacted managed fingerprint only.

This repository is not an Atlantis production release yet. The corresponding
`homelab-infra/terraform/aiostreams` root remains pending dedicated backend,
provider distribution, identity, recovery and runtime admission.

Reference: https://docs.aiostreams.viren070.me/apis/user/
