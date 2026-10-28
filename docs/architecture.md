# Safety and provider ownership

AIOStreams v2.x has a **whole-document** API at `/api/v1/user`. POST creates a configuration; GET (Basic UUID/password) reads it; PUT replaces it. Profiles/personas, addons, presets and services therefore belong to one Terraform resource, not several writers. The `DELETE` endpoint is irreversible and must NOT be called implicitly on resource destroy.

API credentials and all configured addon URLs may be secrets. No credentials in Git, state, logs, diagnostics or PR output. Any future Terraform schema must use ephemeral/write-only input where supported. An ordinary `Sensitive` attribute is **not** safe from state storage.

AIOStreams `JELLYFIN_MAX_PERSONAS` and server-level defaults are environment/runtime configuration managed by Kubernetes GitOps, not by this client library.

Persona `lock` values are native AIOStreams PINs. The API accepts the initial
numeric PIN and hashes it during persistence; raw UserData reads return the
stored bcrypt hash only. The provider therefore accepts `lock_wo` together
with `lock_wo_version`: the write-only value is applied only on the first
write or an explicit version increment, while later updates retain the
remote hash. This prevents a user/admin PIN change from being reverted by an
unrelated Terraform update. `JELLYFIN_PIN_SIGN_IN=false` remains a server
runtime setting, so the shared configuration password is always required for
the family picker. v2.37.0 has no native self-service endpoint for a persona
to change its own PIN.

The implemented resource targets the deployed v2.37.0 UserData contract. It
uses write-only OpenTofu attributes for passwords and credentials, reads raw
UserData, overlays managed fields and retains unknown fields. Read operations
use `AIOSTREAMS_PASSWORD` from the execution environment because write-only
values are unavailable in the framework Read request. The resource never
calls DELETE. The Atlantis root remains pending until its dedicated S3
backend, lock identity, provider package, recovery drill and isolated runtime
qualification are complete.
