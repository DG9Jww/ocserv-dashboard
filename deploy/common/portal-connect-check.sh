#!/bin/sh
# Ocserv supplies USERNAME after either authentication method succeeds.
set -eu
case "${USERNAME:-}" in
    ''|.|..|*[!A-Za-z0-9._-]*) exit 1 ;;
esac

gate="/etc/ocserv/managed-customers/${USERNAME}"
[ -f "${gate}" ] || exit 1
IFS= read -r expires <"${gate}" || exit 1
case "${expires}" in
    ''|*[!0-9]*) exit 1 ;;
esac
if [ "${expires}" -ne 0 ]; then
    [ "$(date +%s)" -lt "${expires}" ] || exit 1
fi

awk -F: -v username="${USERNAME}" '
    $1 == username {
        entries++
        if (NF == 3 && $3 != "" && $3 !~ /^[!*]/) valid++
    }
    END { exit !(entries == 1 && valid == 1) }
' /etc/ocserv/ocpasswd
