FROM alpine:3.21

# Keep this image package-only: Alpine already provides a POSIX shell and wget.
RUN apk add --no-cache \
    bind-tools \
    ca-certificates \
    curl \
    iproute2 \
    iputils \
    jq \
    netcat-openbsd \
    openssl \
    tcpdump \
    yq-go
