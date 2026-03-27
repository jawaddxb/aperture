# Aperture Deployment Guide

## Local Development

```bash
# Clone
git clone https://github.com/jawaddxb/aperture
cd aperture

# Build
go build -o aperture-server ./cmd/aperture-server

# Configure
cp aperture.yaml.example aperture.yaml
# Edit aperture.yaml with your LLM key and Chrome path

# Run
./aperture-server

# Or with hot reload
make run
```

## Docker

### Build

```bash
docker build -t aperture:latest -f deploy/Dockerfile .
```

### Run

```bash
docker run -d \
  --name aperture \
  -p 8080:8080 \
  -v $(pwd)/aperture.yaml:/app/aperture.yaml \
  aperture:latest
```

### Docker Compose

```bash
docker compose -f deploy/docker-compose.yml up -d
```

## Railway

### Prerequisites
- Railway CLI installed (`npm i -g @railway/cli`)
- GitHub repo connected to Railway

### Deploy Steps

1. **Create project:**
   ```bash
   railway init
   railway link
   ```

2. **Set environment variables:**
   ```bash
   railway variables set APERTURE_BROWSER_CHROMIUM_PATH=/usr/bin/chromium-browser
   railway variables set APERTURE_LLM_API_KEY=sk-or-v1-xxx
   railway variables set APERTURE_LLM_BASE_URL=https://openrouter.ai/api
   railway variables set APERTURE_LLM_MODEL=openai/gpt-4o-mini
   railway variables set APERTURE_API_REQUIRE_AUTH=true
   railway variables set APERTURE_API_KEYS=apt_your_production_key
   ```

3. **Deploy:**
   ```bash
   railway up
   ```

4. **Verify:**
   ```bash
   curl https://your-app.up.railway.app/health
   curl https://your-app.up.railway.app/api/v1/bridge/health \
     -H "Authorization: Bearer apt_your_production_key"
   ```

### Railway Notes
- Railway auto-detects Go and runs `go build`
- Set `NIXPACKS_BUILD_CMD` if you need custom build steps
- Chromium needs to be installed — use a Dockerfile or nixpacks config
- The Scrapling Python fallback requires Python 3.10+ in the container

## PM2 (Local Server)

```bash
# Start
pm2 start ./aperture-server --name aperture-server

# Save for reboot persistence
pm2 save
pm2 startup

# Monitor
pm2 logs aperture-server
pm2 monit
```

## Production Checklist

- [ ] `api.require_auth: true` — never run open in production
- [ ] API keys generated with `apt_` prefix
- [ ] `api.rate_limit_rpm` set (e.g., 120 for 2 req/sec)
- [ ] `cors_origins` restricted to your domains
- [ ] `llm.api_key` set via environment variable, not YAML
- [ ] `log.level: "warn"` or `"error"` for production
- [ ] Health check configured: `GET /health`
- [ ] Reverse proxy (nginx/Caddy) with TLS termination
- [ ] Chromium binary available at `browser.chromium_path`
- [ ] Scrapling installed (`pip install "scrapling[all]"` + `scrapling install`)
- [ ] Monitor browser pool exhaustion via `/api/v1/bridge/health`

## Security

- API keys are validated server-side before any browser action
- Cookie persistence files are stored with `0600` permissions
- Proxy credentials support `http://user:pass@host:port` format
- No secrets are logged (API keys are redacted in debug output)
- CORS is configurable per-origin for API access control
