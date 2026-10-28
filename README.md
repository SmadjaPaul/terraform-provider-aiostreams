# terraform-provider-aiostreams

OpenTofu/Terraform provider for AIOStreams' native `/api/v1/user` configuration API.

The provider is being developed against the versioned API contract. Never commit configuration passwords, manifest URLs or TorBox API keys. Configuration writes replace the whole document; a resource must own it as a single unit. Destructive operations require an explicit opt-in.
