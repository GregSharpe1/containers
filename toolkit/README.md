# toolkit

Small Alpine-based toolkit for DevOps and platform troubleshooting.

Included tools:

- HTTP and TLS: `curl`, `openssl`
- DNS: `dig`, `nslookup`, `host`
- Networking: `ip`, `ping`, `tracepath`, `nc`, `tcpdump`
- Structured data: `jq`, `yq`

Alpine's built-in POSIX shell and `wget` are also available. The image deliberately uses Alpine packages only, avoiding large vendor CLIs so it remains compact and supports both `linux/amd64` and `linux/arm64`.

## Local Build

```bash
make build IMAGE=toolkit
```

## Local Run

```bash
make run IMAGE=toolkit
```

## Published Image

`ghcr.io/gregsharpe1/toolkit`
