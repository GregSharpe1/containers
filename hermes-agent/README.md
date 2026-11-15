# hermes-agent

Extends `nousresearch/hermes-agent` with pinned `kubectl` and GitHub CLI (`gh`) clients for read-only Kubernetes investigations and GitHub-based workflows.

The image downloads kubectl and GitHub CLI binaries for the target architecture, verifies their published SHA-256 checksums, and runs command smoke checks during the image build.

## Local Build

```bash
make build IMAGE=hermes-agent
```

## Published Image

`ghcr.io/gregsharpe1/hermes-agent`
