# hermes-agent

Extends `nousresearch/hermes-agent` with pinned Kubernetes, GitHub, infrastructure, JSON, Python, and Terraform version-management CLIs: `kubectl`, `gh`, `helm`, `kustomize`, `jq`, `uv`, and `tfenv`.

The image downloads architecture-specific binaries, verifies published SHA-256 checksums where available, installs tfenv at a pinned tag, and runs command smoke checks during the image build.

## Local Build

```bash
make build IMAGE=hermes-agent
```

## Published Image

`ghcr.io/gregsharpe1/hermes-agent`
