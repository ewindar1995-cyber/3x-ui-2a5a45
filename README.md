# 3x-ui on Back4app Containers

A self-hosted [3x-ui](https://github.com/MHSanaei/3x-ui) panel (the Xray-core
management panel) running on a free Back4app Container.

A small Go launcher multiplexes the panel, the subscription server and the
VLESS WebSocket traffic over the single public port by HTTP path:

| Public path | Internal target | Purpose |
|---|---|---|
| `/panel/` (configurable) | panel (port `XUI_PORT`) | Admin web panel |
| `/sub/` `/json/` `/clash/` | subscription server (port `SUB_PORT`) | Subscription links |
| `/ws/` (configurable) | VLESS+WebSocket inbound (port `VLESS_PORT`) | Proxy traffic |
| `/` and `/healthz` | — | Health check |

## Deploy

1. Push this folder to a GitHub repository.
2. On [Back4app](https://www.back4app.com/), create a new **Containers** app,
   connect your GitHub repo, and click **Create App**.
3. (Optional) Set these environment variables before deploying:
   - `USERNAME` (default `admin`) — panel username
   - `PASSWORD` — panel password (auto-generated if empty)
   - `ADMIN_PATH` (default `/panel/`) — hidden panel path
   - `WS_PREFIX` (default `/ws/`) — WebSocket path

## Creating a VLESS inbound

1. Open `https://<your-app>.back4app.app<ADMIN_PATH>` and log in.
2. **Inbounds → Add Inbound**:
   - Protocol: `vless`
   - Port: `20868` (must match `VLESS_PORT`)
   - Client: generate a UUID
   - Stream Settings → Network: `ws`
   - Security: `none`
   - ws Settings → Path: `/ws/` (must match `WS_PREFIX`)
3. Save, then copy the client's subscription link into your client
   (v2rayNG, Streisand, Hiddify, Nekoray...).

## Notes

- The free tier gives ~256 MB RAM. The panel is configured to skip fail2ban
  to stay within limits, but very heavy use may still be tight.
- The free tier is ephemeral: panel config (inbounds/clients) resets when the
  app sleeps or redeploys. Credentials persist via environment variables.
- Only HTTP-based transports (WebSocket, XHTTP) work here. Raw TCP and REALITY
  are not compatible with this architecture.
