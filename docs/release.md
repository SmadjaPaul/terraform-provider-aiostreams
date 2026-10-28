# Provider release gate

The provider is intended for the public Terraform Registry at
`registry.terraform.io/smadjapaul/aiostreams`.

The release workflow is tag-driven and produces signed multi-platform
GoReleaser archives, SHA-256 checksums and the Terraform Registry manifest.
Before pushing the first tag, configure the repository's `GPG_PRIVATE_KEY` and
`PASSPHRASE` secrets and register the public signing key with the Terraform
Registry. The registry publication itself is a separate account-level action;
a GitHub Release alone is not sufficient for `tofu init`.

The first immutable release tag is intended to be `v0.1.0`. After the registry
reports the provider version available, regenerate the infrastructure root
lockfile from the published Linux AMD64 and Atlantis-supported packages. Do
not reuse the local qualification hashes as production provenance.
