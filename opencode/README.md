# opencode

Extends `ghcr.io/anomalyco/opencode` and adds `kubectl`, `gh`, Docker CLI, and Node.js/npm for personal tooling and npx-based MCP servers.

## Docker Builds

The image includes the Docker CLI, but intentionally does not run a Docker daemon. To build images from an OpenCode container, provide either a Docker socket or a remote Docker/BuildKit endpoint.

With a host Docker daemon:

```bash
docker run --rm -it \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$PWD":/workspace -w /workspace \
  ghcr.io/gregsharpe1/opencode:latest sh
```

Then run `docker build`, `docker login`, and `docker push` inside the container. The mounted socket grants control of the host container runtime, so use this only for a trusted OpenCode instance.

For a remote daemon or BuildKit service, set `DOCKER_HOST` in the container environment instead of mounting `/var/run/docker.sock`.

## Local Build

```bash
make build IMAGE=opencode
```

## Local Run

```bash
make run IMAGE=opencode
```

## Published Image

`ghcr.io/gregsharpe1/opencode`
