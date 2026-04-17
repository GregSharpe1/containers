FROM alpine:3.21 AS kubectl-downloader

ARG KUBECTL_VERSION=v1.35.0
ARG TARGETARCH

RUN apk add --no-cache curl && \
    curl -fsSL -o /usr/local/bin/kubectl "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" && \
    chmod +x /usr/local/bin/kubectl

FROM ghcr.io/anomalyco/opencode:1.4.9

COPY --from=kubectl-downloader /usr/local/bin/kubectl /usr/local/bin/kubectl
