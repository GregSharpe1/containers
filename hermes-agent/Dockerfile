FROM alpine:3.21 AS kubectl-downloader

# renovate: datasource=github-tags depName=kubernetes/kubernetes
ARG KUBECTL_VERSION=v1.34.1
# renovate: datasource=github-releases depName=cli/cli
ARG GH_VERSION=2.90.0
ARG TARGETARCH
RUN apk add --no-cache ca-certificates curl tar && \
    curl -fsSL -o /tmp/kubectl "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" && \
    curl -fsSL -o /tmp/kubectl.sha256 "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl.sha256" && \
    printf '%s  %s\n' "$(cat /tmp/kubectl.sha256)" /tmp/kubectl | sha256sum -c - && \
    install -m 0755 /tmp/kubectl /usr/local/bin/kubectl && \
    case "${TARGETARCH}" in \
      amd64) gh_arch='amd64' ;; \
      arm64) gh_arch='arm64' ;; \
      *) printf 'Unsupported TARGETARCH: %s\n' "${TARGETARCH}" >&2; exit 1 ;; \
    esac && \
    gh_archive="gh_${GH_VERSION}_linux_${gh_arch}.tar.gz" && \
    curl -fsSL -o "/tmp/${gh_archive}" "https://github.com/cli/cli/releases/download/v${GH_VERSION}/${gh_archive}" && \
    curl -fsSL -o /tmp/gh_checksums.txt "https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_checksums.txt" && \
    cd /tmp && grep "  ${gh_archive}$" gh_checksums.txt | sha256sum -c - && \
    tar -xzf "/tmp/${gh_archive}" && \
    install -m 0755 "/tmp/gh_${GH_VERSION}_linux_${gh_arch}/bin/gh" /usr/local/bin/gh

FROM nousresearch/hermes-agent:v0.23.1

COPY --from=kubectl-downloader /usr/local/bin/kubectl /usr/local/bin/kubectl

# Fail the image build if either the base agent or the added client is unusable.
RUN hermes --help >/dev/null && kubectl version --client=true && gh --version
