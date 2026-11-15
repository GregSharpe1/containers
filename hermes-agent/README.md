# hermes-agent

Extends `nousresearch/hermes-agent` with a pinned `kubectl` client for read-only Kubernetes investigations.

The image downloads the kubectl binary for the target architecture, verifies its published SHA-256 checksum, and runs command smoke checks during the image build.

## Local Build

```bash
make build IMAGE=hermes-agent
```

## Published Image

`ghcr.io/gregsharpe1/hermes-agent`
