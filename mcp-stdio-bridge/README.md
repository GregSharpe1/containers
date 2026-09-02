# mcp-stdio-bridge

`mcp-stdio-bridge` packages Supergateway with the pinned Slack and Argo CD MCP
servers used by the Kubernetes MCP gateway. It exposes the server selected by
the container arguments through Streamable HTTP.

Published image:

```text
ghcr.io/gregsharpe1/mcp-stdio-bridge
```

Included packages:

- `supergateway@3.4.3`
- `slack-mcp-server@1.3.0`
- `argocd-mcp@0.9.0`

Build locally:

```bash
make build IMAGE=mcp-stdio-bridge
```
