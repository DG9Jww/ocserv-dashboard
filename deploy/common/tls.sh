#!/usr/bin/env bash
# Resolves the server certificate used by ocserv and nginx.
# Sets and exports SERVER_CERT / SERVER_KEY.

SELF_SIGNED_CERT=/etc/ocserv/certs/cert.pem
SELF_SIGNED_KEY=/etc/ocserv/certs/cert.key

resolve_tls_paths() {
    local cert="${TLS_CERT_FILE:-}"
    local key="${TLS_KEY_FILE:-}"
    local host="${HOST:-}"

    cert="${cert//\{HOST\}/${host}}"
    key="${key//\{HOST\}/${host}}"

    if [[ -z "${cert}" && -z "${key}" ]]; then
        SERVER_CERT="${SELF_SIGNED_CERT}"
        SERVER_KEY="${SELF_SIGNED_KEY}"
    elif [[ -r "${cert}" && -r "${key}" ]]; then
        SERVER_CERT="${cert}"
        SERVER_KEY="${key}"
    else
        printf 'TLS_CERT_FILE/TLS_KEY_FILE are set but not readable: %s %s\n' "${cert}" "${key}" >&2
        return 1
    fi
    export SERVER_CERT SERVER_KEY
}
