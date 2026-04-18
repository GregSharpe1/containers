FROM alpine:3.21 AS tools-downloader

ARG KUBECTL_VERSION=v1.35.0
ARG GH_VERSION=2.90.0
ARG TARGETARCH

RUN apk add --no-cache curl tar && \
    curl -fsSL -o /usr/local/bin/kubectl "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" && \
    chmod +x /usr/local/bin/kubectl && \
    case "${TARGETARCH}" in \
      amd64) gh_arch='amd64' ;; \
      arm64) gh_arch='arm64' ;; \
      *) printf 'Unsupported TARGETARCH: %s\n' "${TARGETARCH}" >&2; exit 1 ;; \
    esac && \
    curl -fsSL -o /tmp/gh.tar.gz "https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_${gh_arch}.tar.gz" && \
    tar -xzf /tmp/gh.tar.gz -C /tmp && \
    cp "/tmp/gh_${GH_VERSION}_linux_${gh_arch}/bin/gh" /usr/local/bin/gh && \
    chmod +x /usr/local/bin/gh

FROM ghcr.io/anomalyco/opencode:1.4.9

# Bun loads native extensions at runtime that expect the glibc loader path.
RUN apk add --no-cache gcompat libstdc++

COPY --from=tools-downloader /usr/local/bin/kubectl /usr/local/bin/kubectl
COPY --from=tools-downloader /usr/local/bin/gh /usr/local/bin/gh
