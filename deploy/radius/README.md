# Central customer authentication with FreeRADIUS 3

The Dashboard Master stores customer accounts, bcrypt password hashes and node grants. FreeRADIUS accepts RADIUS requests from Ocserv and asks Master to check the account and grant. Ocserv uses the same credentials as the customer portal. FreeRADIUS does not need SQL access or a separate copy of customer credentials.

## Prepare the deployment

1. Back up the Master database, each node's Ocserv configuration and the FreeRADIUS configuration. Review the VPN maintenance window. This switches the participating Ocserv listener to RADIUS password authentication. Create and grant unified customer accounts in **Customer Management** for any standalone accounts that still need access. Existing unified customers keep their current passwords. Legacy certificates and local accounts cannot authenticate on nodes switched to RADIUS. Their existing portal history remains accessible until the customer is permanently deleted.
2. Use Ocserv 1.6.0 or newer with radcli and `Acct-Session-Id` support in Access-Request. Configure Master and each participating Agent with `CUSTOMER_MULTI_NODE_ENABLED=true`, `CUSTOMER_AUTH_MODE=radius` and `OCSERV_REGENERATE_CONFIG=false`. Keep the existing HTTPS settings for the Master-to-Agent service API.
3. Set `CUSTOMER_RADIUS_REST_SECRET` in Master's `.env` to an independent secret generated with `openssl rand -hex 32`. Docker `setup.sh` publishes the dedicated callback on host loopback at `127.0.0.1:18120`. The systemd backend binds that address directly. The callback does not use port 3000 or Cloudflare.
4. On the FreeRADIUS host, install the REST module for your distribution, such as `freeradius-rest` on Debian. Install [the module template](ocserv-dashboard.rest) under `mods-available/ocserv_dashboard` and [the virtual server template](ocserv-dashboard.site) under `sites-available/ocserv_dashboard`. Set the same REST secret in both module sections and keep the files readable only by the service and administrators. Enable both using symlinks under `mods-enabled` and `sites-enabled`.
5. Bind the virtual server to a private address reachable by all participating Ocserv instances. The example uses UDP 18122 for authentication and 18123 for accounting to coexist with FreeRADIUS's default server. Register the actual source IP FreeRADIUS sees from each instance. Use shortname `master` for Master and `agent-<id>` for each Agent, with the numeric Dashboard Agent ID. Give each client a separate random RADIUS shared secret. The virtual server replaces the incoming NAS identifier with the configured client shortname, so each node must connect from a distinct trusted source IP. Use a private network or an encrypted tunnel between nodes and firewall the UDP ports to those IPs.
6. On each node, copy the installed radcli configuration into the persistent Ocserv directory, for example `/etc/ocserv/radiusclient.conf`. Set `authserver` to `<private-radius-ip>:18122` and `acctserver` to `<private-radius-ip>:18123`. Point `servers` to a protected file in `/etc/ocserv` containing `<private-radius-ip> <that-node-shared-secret>`. Keep the radcli dictionary reference from the installed package. On Docker, place the files in the host directory mounted at `/etc/ocserv` (by default `/opt/ocserv_dashboard/docker_volumes/ocserv`).
7. In each node's persistent `ocserv.conf`, replace existing `auth` and `acct` directives with the following. Remove every `enable-auth` directive and the Dashboard's `portal-connect-check` hook. Keep the VPN server's TLS certificate and private key settings. Use the default virtual host. For custom Ocserv groups, configure the names in the corresponding node's `select-group` and group profiles.

   ```ini
   auth = "radius[config=/etc/ocserv/radiusclient.conf,groupconfig=true]"
   acct = "radius[config=/etc/ocserv/radiusclient.conf]"
   stats-report-time = 60
   ```

8. Run `freeradius -XC` with your distribution's service configuration before reloading FreeRADIUS. Use the same Master and Agent names in Dashboard grants and the client map. Upgrade Master and each Agent using their usual deployment command during the maintenance window. For a Docker Master, use `./setup.sh --deployment docker --node master --yes` to apply the image, environment and port mapping. A Docker restart then preserves the persistent Ocserv configuration.

## Verify

- Create a node group with two members and assign it to a unified customer. Sign into `/customer/` and connect to both VPN hosts using the same credentials. Confirm that no matching local entry was created in `ocpasswd` or Ocserv Users.
- Check that wrong passwords and non-granted nodes fail. Change the password in the portal and confirm that fresh VPN logins use it. Switch the customer from the group to one of those nodes and verify that only the selected node still accepts the account. Lock the account and verify that all granted nodes reject fresh authentication and the Master retries disconnection if a node is unreachable.
- Transfer traffic and check the portal after an accounting update. Monthly quotas use calendar months in UTC. Quota enforcement sees traffic as interim updates arrive. The Dashboard records RADIUS session start, interim and stop events. Legacy node records remain separate from this accounting.
- Verify a customer with a configured expiry. The RADIUS node's expiry days are counted from the unified account's creation date. Master sends the remaining time to Ocserv as `Session-Timeout`. Review the node defaults and expiry dates before moving existing customers.
- Certificate downloads are unavailable in RADIUS mode. The customer portal displays the default Cisco connection and the VPN client prompts for credentials.

See [Ocserv's RADIUS instructions](https://gitlab.com/openconnect/ocserv/-/blob/master/doc/README-radius.md) and the [FreeRADIUS REST module configuration](https://raw.githubusercontent.com/FreeRADIUS/freeradius-server/v3.2.x/raddb/mods-available/rest).
