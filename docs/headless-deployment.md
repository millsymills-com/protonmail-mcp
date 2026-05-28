# Headless Linux deployment

Run `protonmail-mcp` as a loopback SSE service with a file credential backend.

The SSE endpoint exposes the authenticated Proton session, so it always
requires a bearer token (`PROTONMAIL_MCP_SSE_TOKEN`, at least 16 characters).
Generate one and keep it secret — every MCP client must send it as
`Authorization: Bearer <token>`:

```bash
openssl rand -hex 32   # use the output as PROTONMAIL_MCP_SSE_TOKEN
```

`PROTONMAIL_MCP_HOST` defaults to `127.0.0.1`. The bearer token is enforced on
every request regardless of bind address, but the SDK's DNS-rebinding
(Host-header) protection only applies on a loopback bind. Binding a non-loopback
address (e.g. `0.0.0.0`) logs a warning and leaves the bearer token as the only
access control — terminate TLS at a reverse proxy in front of it.

## 1. One-time bootstrap (interactive, on the box)

Run `login` once as the service user, writing to the file backend. **At the 2FA
prompt, paste the `otpauth://` secret URI (not a 6-digit code)** so the service
can self-heal later:

```bash
sudo -u interceptor-mcp env \
  PROTONMAIL_MCP_CREDENTIAL_BACKEND=file \
  PROTONMAIL_MCP_STATE_DIR=/var/lib/protonmail-mcp \
  protonmail-mcp login
```

Verify: run `PROTONMAIL_MCP_CREDENTIAL_BACKEND=file PROTONMAIL_MCP_STATE_DIR=/var/lib/protonmail-mcp protonmail-mcp status` and confirm it prints `backend: file` with a valid session.

## 2. systemd unit (`/etc/systemd/system/protonmail-mcp.service`)

```ini
[Unit]
Description=protonmail-mcp (headless SSE)
After=network-online.target

[Service]
User=interceptor-mcp
StateDirectory=protonmail-mcp
Environment=PROTONMAIL_MCP_TRANSPORT=sse
Environment=PROTONMAIL_MCP_HOST=127.0.0.1
Environment=PROTONMAIL_MCP_PORT=8770
Environment=PROTONMAIL_MCP_CREDENTIAL_BACKEND=file
# Keep the token out of the unit file; load it from a 0600 file owned by root.
EnvironmentFile=/etc/protonmail-mcp/sse-token.env
ExecStart=/usr/local/bin/protonmail-mcp
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

`StateDirectory=protonmail-mcp` makes systemd provide `/var/lib/protonmail-mcp`
(0700, owned by the service user) and sets `$STATE_DIRECTORY` — so the bootstrap
in step 1 and the service resolve the same path.

## 3. Self-heal and its limit

If the refresh token is revoked, the service re-logins from the stored
credentials (TOTP generated from the stored secret) on the next call. **If Proton
answers with a CAPTCHA challenge, it cannot self-heal** — re-run step 1 manually.
