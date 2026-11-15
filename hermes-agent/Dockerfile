FROM alpine:3.21 AS kubectl-downloader

ARG KUBECTL_VERSION=v1.34.1
ARG TARGETARCH

# renovate: datasource=github-tags depName=kubernetes/kubernetes
RUN apk add --no-cache ca-certificates curl && \
    curl -fsSL -o /tmp/kubectl "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" && \
    curl -fsSL -o /tmp/kubectl.sha256 "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl.sha256" && \
    cd /tmp && sha256sum -c kubectl.sha256 && \
    install -m 0755 kubectl /usr/local/bin/kubectl

FROM nousresearch/hermes-agent:v0.23.1

COPY --from=kubectl-downloader /usr/local/bin/kubectl /usr/local/bin/kubectl

# Fail the image build if either the base agent or the added client is unusable.
RUN hermes --help >/dev/null && kubectl version --client=true
