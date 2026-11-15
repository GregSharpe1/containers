# Agent Instructions

## Repository purpose

This repository contains Docker image definitions published to `ghcr.io/gregsharpe1`.
Each top-level directory containing a `Dockerfile` is a publishable image.

## Before making changes

- Read `README.md`, the affected image's `README.md`, `metadata.env`, and the relevant GitHub Actions workflow.
- Inspect the current worktree and recent history; preserve unrelated changes.
- Keep changes focused and follow the existing image-directory layout.
- Do not add credentials, tokens, private keys, or secret-derived configuration.

## Image requirements

- Each image directory must contain a `Dockerfile`, `.dockerignore`, `README.md`, and `metadata.env`.
- Pin base images and downloaded tool versions explicitly.
- Add Renovate metadata for manually downloaded tools so their versions remain managed.
- For architecture-dependent downloads, use BuildKit's `TARGETARCH` and fail on unsupported architectures.
- Verify downloaded binaries when the upstream project publishes checksums.
- Keep image documentation and `metadata.env` aligned with the Dockerfile.

## Validation

Run the narrowest relevant checks before opening a pull request:

- `python3 -m json.tool renovate.json`
- `npx --yes --package renovate renovate-config-validator renovate.json`
- `git diff --check`
- Build the changed image with `make build IMAGE=<name>` when Docker is available.
- Include executable smoke checks in the Dockerfile for newly installed command-line tools.

If a local Docker daemon or another required dependency is unavailable, report that limitation; do not claim the image build passed.

## Pull-request hygiene

- Use a branch from the current `main` branch.
- Keep documentation or agent-policy changes in a separate pull request from image implementation changes.
- Do not merge pull requests unless explicitly requested.
- Report the pull-request URL, changed scope, validation results, and any unavailable checks.

## Publishing and rollout

- Pull requests build changed images without publishing them.
- Merges to `main` publish images tagged with the short Git SHA and `latest`.
- Updating a Kubernetes workload to consume a published image is a separate GitOps change; a successful image build is not rollout evidence.
