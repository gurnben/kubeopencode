# Crush Tool Init Container
#
# This minimal image provides the Crush CLI binary for copying to shared volumes.
# Used as an init container to provision Crush to work containers.
#
# Architecture:
#   Init Container (this image) -> copies -> /tools/crush -> Work Container uses it
#
# Usage:
#   # As init container in Kubernetes Pod:
#   initContainers:
#     - name: crush-init
#       image: ghcr.io/gurnben/crush-container:latest
#       volumeMounts:
#         - name: tools
#           mountPath: /tools
#   containers:
#     - name: agent
#       image: ghcr.io/kubeopencode/kubeopencode-agent-devbox:latest
#       command: ["/tools/crush", "run", "--yolo", "..."]
#       volumeMounts:
#         - name: tools
#           mountPath: /tools
#
# Binary Source:
#   Downloaded from GitHub Releases: https://github.com/charmbracelet/crush/releases
#   Crush is built with CGO_ENABLED=0 — fully static, no glibc dependency.
#
# Version Management:
#   CRUSH_VERSION must be passed via --build-arg.

FROM debian:bookworm-20260316-slim

ARG CRUSH_VERSION

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

ARG TARGETARCH
RUN set -ex; \
    case "${TARGETARCH}" in \
        amd64) ARCH="x86_64" ;; \
        arm64) ARCH="arm64" ;; \
        *) echo "Unsupported architecture: ${TARGETARCH}" && exit 1 ;; \
    esac; \
    FILENAME="crush_${CRUSH_VERSION}_Linux_${ARCH}.tar.gz"; \
    URL="https://github.com/charmbracelet/crush/releases/download/v${CRUSH_VERSION}/${FILENAME}"; \
    echo "Downloading Crush v${CRUSH_VERSION} from ${URL}..."; \
    curl --retry 5 --retry-delay 3 --retry-all-errors -fsSL "${URL}" -o /tmp/crush.tar.gz; \
    tar -xzf /tmp/crush.tar.gz -C /tmp; \
    mv /tmp/crush_${CRUSH_VERSION}_Linux_${ARCH}/crush /crush; \
    chmod +x /crush; \
    rm -f /tmp/crush.tar.gz; \
    rm -rf /tmp/crush_*; \
    /crush --version

RUN apt-get purge -y curl && apt-get autoremove -y && rm -rf /var/lib/apt/lists/*

ENV TOOLS_DIR=/tools
ENV CRUSH_BIN=crush

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
