FROM alpine:3.21 AS tooling-downloader

# renovate: datasource=github-tags depName=kubernetes/kubernetes
ARG KUBECTL_VERSION=v1.34.1
# renovate: datasource=github-releases depName=cli/cli
ARG GH_VERSION=2.90.0
# renovate: datasource=github-releases depName=helm/helm
ARG HELM_VERSION=4.3.2
# renovate: datasource=github-releases depName=kubernetes-sigs/kustomize
ARG KUSTOMIZE_VERSION=5.8.3
# renovate: datasource=github-releases depName=jqlang/jq
ARG JQ_VERSION=1.8.2
# renovate: datasource=github-releases depName=astral-sh/uv
ARG UV_VERSION=0.13.8
# renovate: datasource=github-tags depName=tfutils/tfenv
ARG TFENV_VERSION=3.2.2
ARG TARGETARCH

RUN apk add --no-cache ca-certificates curl git tar && \
    case "${TARGETARCH}" in \
      amd64) gh_arch='amd64'; uv_arch='x86_64-unknown-linux-gnu' ;; \
      arm64) gh_arch='arm64'; uv_arch='aarch64-unknown-linux-gnu' ;; \
      *) printf 'Unsupported TARGETARCH: %s\n' "${TARGETARCH}" >&2; exit 1 ;; \
    esac && \
    uv_dir="uv-${uv_arch}" && \
    gh_archive="gh_${GH_VERSION}_linux_${gh_arch}.tar.gz" && \
    helm_archive="helm-v${HELM_VERSION}-linux-${TARGETARCH}.tar.gz" && \
    kustomize_archive="kustomize_v${KUSTOMIZE_VERSION}_linux_${TARGETARCH}.tar.gz" && \
    jq_asset="jq-linux-${TARGETARCH}" && \
    uv_archive="uv-${uv_arch}.tar.gz" && \
    curl -fsSL -o /tmp/kubectl "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl" && \
    curl -fsSL -o /tmp/kubectl.sha256 "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${TARGETARCH}/kubectl.sha256" && \
    printf '%s  %s\n' "$(cat /tmp/kubectl.sha256)" /tmp/kubectl | sha256sum -c - && \
    install -m 0755 /tmp/kubectl /usr/local/bin/kubectl && \
    curl -fsSL -o "/tmp/${gh_archive}" "https://github.com/cli/cli/releases/download/v${GH_VERSION}/${gh_archive}" && \
    curl -fsSL -o /tmp/gh_checksums.txt "https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_checksums.txt" && \
    grep "  ${gh_archive}$" /tmp/gh_checksums.txt | sed "s#  ${gh_archive}#  /tmp/${gh_archive}#" | sha256sum -c - && \
    tar -xzf "/tmp/${gh_archive}" -C /tmp && \
    install -m 0755 "/tmp/gh_${GH_VERSION}_linux_${gh_arch}/bin/gh" /usr/local/bin/gh && \
    curl -fsSL -o "/tmp/${helm_archive}" "https://get.helm.sh/${helm_archive}" && \
    curl -fsSL -o /tmp/helm.sha256sum "https://get.helm.sh/${helm_archive}.sha256sum" && \
    printf '%s  %s\n' "$(awk '{print $1}' /tmp/helm.sha256sum)" "/tmp/${helm_archive}" | sha256sum -c - && \
    tar -xzf "/tmp/${helm_archive}" -C /tmp && \
    install -m 0755 "/tmp/linux-${TARGETARCH}/helm" /usr/local/bin/helm && \
    curl -fsSL -o "/tmp/${kustomize_archive}" "https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize/v${KUSTOMIZE_VERSION}/${kustomize_archive}" && \
    curl -fsSL -o /tmp/kustomize_checksums.txt "https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize/v${KUSTOMIZE_VERSION}/checksums.txt" && \
    grep "  ${kustomize_archive}$" /tmp/kustomize_checksums.txt | sed "s#  ${kustomize_archive}#  /tmp/${kustomize_archive}#" | sha256sum -c - && \
    mkdir /tmp/kustomize && tar -xzf "/tmp/${kustomize_archive}" -C /tmp/kustomize && \
    install -m 0755 /tmp/kustomize/kustomize /usr/local/bin/kustomize && \
    curl -fsSL -o /tmp/jq "https://github.com/jqlang/jq/releases/download/jq-${JQ_VERSION}/jq-linux-${TARGETARCH}" && \
    curl -fsSL -o /tmp/jq_checksums.txt "https://github.com/jqlang/jq/releases/download/jq-${JQ_VERSION}/sha256sum.txt" && \
    grep "  ${jq_asset}$" /tmp/jq_checksums.txt | sed "s#  ${jq_asset}#  /tmp/jq#" | sha256sum -c - && \
    install -m 0755 /tmp/jq /usr/local/bin/jq && \
    curl -fsSL -o "/tmp/${uv_archive}" "https://github.com/astral-sh/uv/releases/download/${UV_VERSION}/${uv_archive}" && \
    curl -fsSL -o /tmp/uv.sha256 "https://github.com/astral-sh/uv/releases/download/${UV_VERSION}/${uv_archive}.sha256" && \
    printf '%s  %s\n' "$(awk '{print $1}' /tmp/uv.sha256)" "/tmp/${uv_archive}" | sha256sum -c - && \
    tar -xzf "/tmp/${uv_archive}" -C /tmp && \
    install -m 0755 "/tmp/${uv_dir}/uv" /usr/local/bin/uv && \
    git clone --depth 1 --branch "v${TFENV_VERSION}" https://github.com/tfutils/tfenv.git /opt/tfenv && \
    chmod +x /opt/tfenv/bin/tfenv

FROM nousresearch/hermes-agent:v0.23.1

ENV TFENV_ROOT=/opt/tfenv

COPY --from=tooling-downloader /usr/local/bin/kubectl /usr/local/bin/kubectl
COPY --from=tooling-downloader /usr/local/bin/gh /usr/local/bin/gh
COPY --from=tooling-downloader /usr/local/bin/helm /usr/local/bin/helm
COPY --from=tooling-downloader /usr/local/bin/kustomize /usr/local/bin/kustomize
COPY --from=tooling-downloader /usr/local/bin/jq /usr/local/bin/jq
COPY --from=tooling-downloader /usr/local/bin/uv /usr/local/bin/uv
COPY --from=tooling-downloader /opt/tfenv /opt/tfenv

RUN ln -s /opt/tfenv/bin/tfenv /usr/local/bin/tfenv && \
    hermes --help >/dev/null && \
    kubectl version --client=true >/dev/null && \
    gh --version >/dev/null && \
    helm version --short >/dev/null && \
    kustomize version >/dev/null && \
    jq --version >/dev/null && \
    uv --version >/dev/null && \
    tfenv --version >/dev/null
