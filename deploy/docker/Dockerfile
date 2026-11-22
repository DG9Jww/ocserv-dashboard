# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.27.1
ARG NODE_VERSION=24
ARG POSTGRES_VERSION=18
ARG AGENT_NODE=false

FROM golang:${GO_VERSION}-bookworm AS backend-builder

ARG AGENT_NODE
ENV CGO_ENABLED=0 \
    GOOS=linux
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN if [ "${AGENT_NODE}" = "true" ]; then export GOFLAGS=-p=1; fi; \
    go build -trimpath -ldflags="-s -w" -o /out/backend ./main.go

FROM node:${NODE_VERSION}-bookworm AS web-builder

ARG CUSTOMER_API_ENABLED=true
ARG ADMIN_BASE_PATH=/

WORKDIR /src
COPY web/admin/ ./web/admin/
COPY web/customer/ ./web/customer/
RUN set -eux; \
        case "${ADMIN_BASE_PATH}" in \
            /) ;; \
            /api|/assets|/customer|/favicon_io|/health|/swagger|''|[!/]*|/*/*|*[!a-zA-Z0-9_/-]*) \
                echo "ADMIN_BASE_PATH must be / or a single non-reserved path segment" >&2; exit 1 ;; \
        esac; \
        admin_prefix="${ADMIN_BASE_PATH%/}"; \
        mkdir -p "/out/admin${admin_prefix}" /out/customer; \
        corepack enable; \
        cd /src/web/admin; \
        yarn install --immutable; \
        VITE_API_BASE_URL="${admin_prefix}/api" \
        VITE_API_TIMEOUT_MS=15000 \
        VITE_I18N_LANGUAGES="en:English,it:Italiano,zh-cn:中文(简体),zh-tw:中文(繁體),ru:Русский,fa:فارسی,ar:العربية" \
        yarn build --base="${admin_prefix}/"; \
        cp -a dist/. "/out/admin${admin_prefix}/"; \
        if [ "${CUSTOMER_API_ENABLED}" = "true" ]; then \
            cd /src/web/customer; \
            yarn install --immutable; \
            VITE_API_BASE_URL=/api \
            VITE_API_TIMEOUT_MS=15000 \
            VITE_USE_MOCKS=false \
            VITE_I18N_LANGUAGES="en:English,it:Italiano,zh-cn:中文(简体),zh-tw:中文(繁體),ru:Русский,fa:فارسی,ar:العربية" \
            yarn build --base=/customer/; \
            cp -a dist/. /out/customer/; \
        fi; 

# PostgreSQL 18 official image is Debian 13 (Trixie) and provides the
# PostgreSQL binaries, gosu, PGDATA layout, and docker-entrypoint.sh.
FROM postgres:${POSTGRES_VERSION}-trixie AS runtime

ARG DEBIAN_FRONTEND=noninteractive
ARG AGENT_NODE

COPY --chmod=0755 deploy/common/ocserv_binary_install.sh /usr/local/bin/ocserv-binary-install
# Ocserv is installed outside of dpkg, so preserve packages providing its linked
# shared libraries before the build dependencies are autoremoved.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        build-essential \
        ca-certificates \
        curl \
        dnsutils \
        git \
        gnutls-bin \
        gperf \
        ipcalc \
        iproute2 \
        iptables \
        jq \
        less \
        libcjose-dev \
        libcurl4-gnutls-dev \
        libev-dev \
        libgnutls28-dev \
        libjansson-dev \
        libkrb5-dev \
        libhttp-parser-dev \
        liblz4-dev \
        libnl-route-3-dev \
        liboath-dev \
        libpam0g-dev \
        libprotobuf-c-dev \
        libradcli-dev \
        libreadline-dev \
        libseccomp-dev \
        libtasn1-bin \
        libtalloc-dev \
        meson \
        ninja-build \
        openssl \
        pkg-config \
        procps \
        protobuf-c-compiler \
        sudo \
        tini \
    && if [ "${AGENT_NODE}" = "true" ]; then export OCSERV_BUILD_JOBS=1; fi \
    && /usr/local/bin/ocserv-binary-install \
    && for binary in occtl ocpasswd; do \
        binary_path="$(command -v "${binary}")"; \
        if [ "${binary_path}" != "/usr/bin/${binary}" ]; then \
            ln -sf "${binary_path}" "/usr/bin/${binary}"; \
        fi; \
    done \
    && runtime_packages="$(ldd /usr/sbin/ocserv /usr/bin/occtl /usr/bin/ocpasswd | awk '/=> \// { print $3 }' | xargs -r -n 1 readlink -f | xargs -r dpkg-query -S | cut -d: -f1 | sort -u)" \
    && apt-mark manual ${runtime_packages} \
    && apt-get purge -y --auto-remove \
        build-essential \
        gperf \
        git \
        libcjose-dev \
        libcurl4-gnutls-dev \
        libev-dev \
        libgnutls28-dev \
        libjansson-dev \
        libkrb5-dev \
        libhttp-parser-dev \
        liblz4-dev \
        libnl-route-3-dev \
        liboath-dev \
        libpam0g-dev \
        libprotobuf-c-dev \
        libradcli-dev \
        libreadline-dev \
        libseccomp-dev \
        libtalloc-dev \
        meson \
        ninja-build \
        pkg-config \
        protobuf-c-compiler \
    && /usr/bin/occtl --version >/dev/null \
    && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/* /usr/local/bin/ocserv-binary-install

WORKDIR /app

COPY --from=backend-builder /out/backend /usr/local/bin/backend
COPY --chmod=755 deploy/docker/entrypoint.sh /usr/local/bin/container-entrypoint
COPY --chmod=755 deploy/docker/server.sh /usr/local/bin/container-server
COPY --chmod=0644 deploy/common/tls.sh /usr/local/lib/ocserv-dashboard/tls.sh

ENV OCSERV_PORT=443 \
    AGENT_NODE=${AGENT_NODE} \
    AGENT_API_HTTPS=false \
    IMAGE_AGENT_NODE=${AGENT_NODE} \
    SYSTEMD=false \
    OC_NET=172.16.24.0/24 \
    OCSERV_DNS=1.1.1.1 \
    SSL_CN=ocserv-dashboard \
    SSL_ORG=ocserv-dashboard \
    SSL_EXPIRE=3650 \
    OCSERV_PRESERVE_CONFIG=false \
    PGDATA=/var/lib/postgresql/18/docker \
    POSTGRES_HOST=127.0.0.1 \
    POSTGRES_PORT=5432 \
    POSTGRES_USER=ocserv \
    POSTGRES_DB=ocserv_db \
    POSTGRES_SSLMODE=disable \
    POSTGRES_READY_MAX_ATTEMPTS=60 \
    POSTGRES_READY_RETRY_SECONDS=1 \
    RUN_MIGRATIONS=true \
    MIGRATION_MAX_ATTEMPTS=30 \
    MIGRATION_RETRY_SECONDS=2 \
    TELEGRAM_RECEIPTS_DIR=/opt/ocserv_dashboard/uploads/receipts \
    OCSERV_DEBUG=999

RUN mkdir -p \
        /app/cron_journal \
        /opt/ocserv_dashboard/uploads/receipts \
        /etc/ocserv/defaults \
        /etc/ocserv/groups \
        /etc/ocserv/users

EXPOSE 443/tcp 443/udp

VOLUME ["/var/lib/postgresql", "/etc/ocserv", "/app/cron_journal", "/opt/ocserv_dashboard/uploads/receipts"]

STOPSIGNAL SIGTERM

ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/container-entrypoint"]
CMD ["/usr/local/bin/container-server"]

FROM runtime AS runtime-true

ARG CUSTOMER_API_ENABLED=false
ARG TELEGRAM_BOT_ENABLED=false
ENV CUSTOMER_API_ENABLED=${CUSTOMER_API_ENABLED} \
    IMAGE_CUSTOMER_API_ENABLED=${CUSTOMER_API_ENABLED} \
    TELEGRAM_BOT_ENABLED=${TELEGRAM_BOT_ENABLED}

EXPOSE 8080/tcp
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=5 \
    CMD flag="$(printf '%s' "$AGENT_API_HTTPS" | tr -d "\"'[:space:]" | tr '[:upper:]' '[:lower:]')"; \
        case "$flag" in \
            true|t|1) curl --fail --silent --insecure https://127.0.0.1:8080/health >/dev/null ;; \
            *) curl --fail --silent http://127.0.0.1:8080/health >/dev/null ;; \
        esac

FROM runtime AS runtime-false

ARG ADMIN_BASE_PATH=/
ARG CUSTOMER_API_ENABLED=true
ARG TELEGRAM_BOT_ENABLED=true
ENV CUSTOMER_API_ENABLED=${CUSTOMER_API_ENABLED} \
    IMAGE_CUSTOMER_API_ENABLED=${CUSTOMER_API_ENABLED} \
    IMAGE_ADMIN_BASE_PATH=${ADMIN_BASE_PATH} \
    TELEGRAM_BOT_ENABLED=${TELEGRAM_BOT_ENABLED}

RUN apt-get update \
    && apt-get install -y --no-install-recommends nginx \
    && rm -rf /var/lib/apt/lists/* \
    && rm -f /etc/nginx/sites-enabled/default \
    && rm -rf /usr/share/nginx/html/*

COPY --from=web-builder /out/admin/ /usr/share/nginx/html/
COPY --from=web-builder /out/customer/ /usr/share/nginx/html/customer/

EXPOSE 3000/tcp 8000/tcp
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=5 \
    CMD curl --fail --silent --insecure https://127.0.0.1:3000/health >/dev/null

# Preserve the installer's AGENT_NODE build argument while keeping the Agent
# build independent of the web-builder and nginx layers.
FROM runtime-${AGENT_NODE} AS final
