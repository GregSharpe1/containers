# containers

Personal Docker image definitions published to `ghcr.io`.

## Repository Layout

Each top-level directory containing a `Dockerfile` is treated as a publishable image.

```text
.
./.github/workflows/
./scripts/
./Makefile
./opencode/
./custom-image-1/
```

Each image directory should contain:

- `Dockerfile`
- `.dockerignore`
- `README.md`
- `metadata.env`

## Local Usage

List available images:

```bash
make list
```

Build one image locally:

```bash
make build IMAGE=opencode
```

Run one image locally:

```bash
make run IMAGE=opencode
```

Build every image:

```bash
make build-all
```

## Publishing

GitHub Actions publishes changed images to `ghcr.io/<owner>/<image-name>`.

- Pull requests validate changed images without pushing.
- Pushes to `main` publish changed images.
- Published tags are the short Git SHA and `latest`.
- `latest` only updates from `main`.

## Adding a New Image

1. Create a new top-level directory with a lowercase kebab-case name.
2. Add `Dockerfile`, `.dockerignore`, `README.md`, and `metadata.env`.
3. Confirm it appears in `make list`.
4. Open a pull request to validate the image build.

## Image Docs

- [`opencode`](./opencode/README.md)
- [`custom-image-1`](./custom-image-1/README.md)
