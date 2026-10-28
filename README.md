# terraform-provider-aiostreams

Early implementation of an AIOStreams OpenTofu provider. **Not yet a usable Terraform provider or production-release candidate.**

Current code: Go client for native `POST/GET/PUT /api/v1/user` and isolated HTTP contract tests. AIOStreams `PUT` is full replacement; delete is intentionally not implemented. No configuration passwords, bearer manifests, service keys or API bodies are logged.

Roadmap: complete Terraform Plugin Framework resource with write-only secrets, secure ID persistence, drift detection, safe JSON ownership, disposable-instance acceptance tests and Atlantis root admission. The API client is intentionally independent of the provider framework.

Reference: https://docs.aiostreams.viren070.me/apis/user/
