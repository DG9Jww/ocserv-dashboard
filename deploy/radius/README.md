# Central customer authentication with FreeRADIUS 3

Master stores customer accounts, password hashes, node access and RADIUS accounting. FreeRADIUS validates requests through Master's REST service. Create customer accounts in **Customer Management**. Each customer selects one node or one RADIUS policy. The customer's quota and expiry are shared across all authorized nodes.

## Deployment

1. Back up your existing databases and Ocserv configuration. For a deployment replacing the earlier certificate-based customer feature, start with fresh databases on Master and Agents. Existing customer data is not migrated.
2. Use Ocserv 1.6.0 or later with radcli and `Acct-Session-Id` in Access-Request. Set `OCSERV_REGENERATE_CONFIG=false` and prepare the following directives in each node's persistent `ocserv.conf`:

   ```ini
   auth = "radius[config=/etc/ocserv/radiusclient.conf,groupconfig=true]"
   acct = "radius[config=/etc/ocserv/radiusclient.conf]"
   stats-report-time = 60
   ```

   Remove `enable-auth`, the old `portal-connect-check` connect script, and active `config-per-user` and `config-per-group` directives. Remove any `ca-cert` and `crl` pointing to the old customer `portal-trust` directory. Keep `server-cert` and `server-key` for VPN TLS. Configure authentication on the default virtual host.
3. On each Ocserv node, set `authserver` to `<private-radius-ip>:18122` and `acctserver` to `<private-radius-ip>:18123` in the radcli configuration. The `servers` option points to a protected file with the RADIUS host and that node's shared secret. Keep the installed radcli dictionary setting. For Docker, put these files under the persistent directory mounted at `/etc/ocserv`.
4. Set a random `CUSTOMER_RADIUS_REST_SECRET` of at least 32 characters in Master's `.env`. Install the [REST module](ocserv-dashboard.rest) and [virtual server](ocserv-dashboard.site) templates in FreeRADIUS. Set the REST module's password in both sections to match Master. Bind the RADIUS site to a private address, use each node's actual source IP, and set client shortnames to `master` and `agent-<Dashboard Agent ID>`. Give each node its own shared secret.
5. Deploy Master and participating Agents. For a Docker Master use `./setup.sh --deployment docker --node master --yes`. The REST callback stays on host loopback TCP port `18120`. Keep the existing HTTPS configuration for the Master–Agent service API and validate FreeRADIUS with `freeradius -XC` before restarting.

## Upgrading from separate node groups and provisioning defaults

Back up the Master database. Before upgrading, manually clear the existing customer accounts and old RADIUS policies. Migration 020 stops if either still contains records. It removes the old node group and provisioning defaults tables and keeps registered Agents, administrator accounts, and node configuration.

## Verification

Create a customer with a single node or a policy, then connect using the customer's password. Verify that unauthorized nodes reject the login and that removing a policy member revokes access and schedules termination there. Set a default connection to enable Cisco Secure Client setup in the portal.

Set a customer quota in bytes and an optional VPN expiry date under **Customer Management**. New customers default to one calendar month from today. The Master stores the end of the selected day in the administrator’s current time zone as a UTC timestamp. Clear the date to remove the expiry. Traffic accounted for across nodes contributes to one quota, including traffic from earlier grants. Monthly usage resets at the beginning of each UTC month. An expired customer can still sign in to `/customer/` to see usage and change their password. Before expiry, the RADIUS reply includes the remaining session time as `Session-Timeout`.

## Customer login protection

In **System → Settings → Customer Login Protection**, a super administrator can adjust the failure threshold, rolling observation window, and lock duration for all customers. The defaults are five consecutive wrong passwords within 15 minutes and a 15-minute lock. Master counts failed password checks from authorized VPN nodes and the customer portal together. A successful sign-in clears the count. Attempts during a lock do not extend it, and existing sessions continue. Use **Customer Management → Clear temporary login lock** to release a customer early. The administrator's account lock is independent.

Migration 021 adds these settings and an optional customer email field without clearing existing data. Email is managed in Customer Management and is currently stored for future use.

On an existing FreeRADIUS 3 installation, add the following line in the `authorize` section's `update request` block of the installed `ocserv_dashboard` virtual server. Keep the existing listener addresses and shared secrets. Run `freeradius -XC` before restarting FreeRADIUS. The field allows Master to recognize retransmissions of the same Access-Request.

```text
&Tmp-String-0 := "%{Packet-Authentication-Vector}"
```

## Radius Policies

Open **System → Radius Policies** on the Master to create and manage shared VPN settings. In **Customer Management**, select a policy or a single node. A policy can be saved with no nodes, which grants no VPN access. Customers assigned directly to a node use that node's network defaults. Clear all customer assignments before deleting a policy.

| Setting | How to configure it |
| --- | --- |
| Primary IPv4 DNS | Enter one IPv4 address, such as `1.1.1.1`. Leave blank to use the node defaults. |
| Secondary IPv4 DNS | Set the primary DNS first, then enter one IPv4 address, such as `1.0.0.1`. Leave blank to omit the secondary override. |
| IPv4 routes | Enter up to 32 IPv4 networks in CIDR notation, one per line, such as `192.168.10.0/24`. Leave blank to use the node routes. |
| Upload limit | Enter a whole number in kB/s for traffic sent by the client, such as `500`. Leave blank or enter `0` to use the node limit. |
| Download limit | Enter a whole number in kB/s for traffic received by the client, such as `2000`. Leave blank or enter `0` to use the node limit. |

One kB is 1,000 bytes. Changes apply at the next full VPN authentication.

The Master sends the policy through the existing FreeRADIUS REST callback. Use `groupconfig=true` on participating nodes. The FreeRADIUS and radcli dictionaries must recognize the Microsoft DNS and Roaring Penguin speed-limit attributes listed in [Ocserv's RADIUS documentation](https://gitlab.com/openconnect/ocserv/-/raw/1.6.0/doc/README-radius.md#L97).
