# uppy-clone

多人网页游戏 — Go backend + frontend multiplayer web game.

## Environment Variables

The following secrets **must be explicitly provided** at deploy time. The
`docker-compose.yml` uses the `${VAR:?VAR required}` syntax, so the stack will
refuse to start when any of them is missing.

| Variable          | Required | Description                                      |
|-------------------|----------|--------------------------------------------------|
| `JWT_SECRET`      | Yes      | HMAC secret used to sign JWTs (>= 32 bytes).     |
| `ADMIN_PASSWORD`  | Yes      | Initial admin password (bcrypt-hashed at boot).  |
| `ENCRYPTION_KEY`  | Yes      | 32-byte hex key for AES-256-GCM field encryption.|

### Generating secure values

```bash
# JWT_SECRET — 32 random bytes as hex
export JWT_SECRET=$(openssl rand -hex 32)

# ENCRYPTION_KEY — 32 random bytes as hex (must be exactly 64 hex chars)
export ENCRYPTION_KEY=$(openssl rand -hex 32)

# ADMIN_PASSWORD — a strong password
export ADMIN_PASSWORD=$(openssl rand -base64 24)
```

> **Security note:** The backend additionally rejects `JWT_SECRET` values
> containing `DEV_ONLY` or `change-in-production` when running in production
> mode (`ENABLE_HSTS != "false"`). Never reuse dev secrets in production.
