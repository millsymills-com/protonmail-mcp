# Headless Linux deployment

Run `protonmail-mcp` as a loopback SSE service with a file credential backend.

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
