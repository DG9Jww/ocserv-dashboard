#!/usr/bin/env bash

# The deployment must not leave another authentication method active beside RADIUS.
radius_ocserv_config_ready() {
    awk '
        /^[[:space:]]*(#|$)/ { next }
        /^[[:space:]]*\[vhost[[:space:]]*:/ { bad = 1 }
        {
            key = $0
            sub(/=.*/, "", key)
            gsub(/^[[:space:]]+|[[:space:]]+$/, "", key)
            value = $0
            sub(/^[^=]*=[[:space:]]*/, "", value)
            sub(/[[:space:]]+#.*$/, "", value)
            sub(/[[:space:]]+$/, "", value)
            if (key == "auth" || key == "acct") {
                count[key]++
                if (value !~ /^"radius\[config=\/[^"]+\]"$/) bad = 1
                if (key == "auth" && value !~ /,groupconfig=true[,\]]/) bad = 1
            }
            if (key == "enable-auth" ||
                (key == "connect-script" && value ~ /portal-connect-check/))
                bad = 1
        }
        END { exit (bad || count["auth"] != 1 || count["acct"] != 1) }
    ' "$1"
}
