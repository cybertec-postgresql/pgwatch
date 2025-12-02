---
title: Running Behind a Reverse Proxy
---

When running pgwatch in production environments, you may want to expose it through a reverse proxy (like Apache, Nginx, or Traefik) on a different path instead of exposing its port directly. This guide shows you how to configure pgwatch for such setups.

## Configuration

### pgwatch Configuration

Use the `--web-base-path` option to specify the base path under which pgwatch should serve its content:

```bash
pgwatch --web-base-path=pgwatch --web-addr=:8432
```

Or using environment variables:

```bash
export PW_WEBBASEPATH=pgwatch
export PW_WEBADDR=:8432
pgwatch
```

The web UI automatically adapts to the configured base path without requiring a rebuild.

### Apache

```apache
<VirtualHost *:443>
    ServerName example.com
    
    # Other SSL and domain configuration...
    
    ProxyPass /pgwatch/ http://localhost:8432/pgwatch/
    ProxyPassReverse /pgwatch/ http://localhost:8432/pgwatch/
    ProxyPreserveHost On
    
    <Location /pgwatch/>
        # Optional: Add authentication
        AuthUserFile /etc/apache2/admpasswd
        AuthType Basic
        AuthName "pgwatch Administration"
        <RequireAll>
            Require valid-user
        </RequireAll>
    </Location>
</VirtualHost>
```

### Nginx

```nginx
server {
    listen 443 ssl;
    server_name example.com;
    
    # Other SSL and domain configuration...
    
    location /pgwatch/ {
        proxy_pass http://localhost:8432/pgwatch/;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Optional: Add authentication
        auth_basic "pgwatch Administration";
        auth_basic_user_file /etc/nginx/.htpasswd;
    }
}
```

## WebSocket Support

The pgwatch web UI uses WebSockets for live log streaming. Make sure your reverse proxy is configured to support WebSocket connections:

### Apache

Requires `mod_proxy_wstunnel`:

```apache
ProxyPass /pgwatch/log ws://localhost:8432/pgwatch/log
ProxyPassReverse /pgwatch/log ws://localhost:8432/pgwatch/log
```

### Nginx

WebSocket support is automatic with the proxy configuration shown above. Nginx will upgrade the connection when needed.

## Testing the Configuration

After configuring your reverse proxy and starting pgwatch:

1. Verify the web UI loads: `https://example.com/pgwatch/`
2. Check that API endpoints work: `https://example.com/pgwatch/readiness`
3. Test WebSocket log streaming in the UI's Logs page
4. Verify that navigation between pages works correctly

## Common Issues

### Static Assets Not Loading

Make sure:

- The reverse proxy's `ProxyPass` directive includes the base path
- Both backend (`--web-base-path`) and frontend use the same path (frontend reads it dynamically from backend)

### WebSocket Connection Failures

Ensure:

- Your reverse proxy supports WebSocket upgrades
- The `/log` endpoint is properly proxied
- No firewall rules are blocking WebSocket connections

### Authentication Issues

If using both reverse proxy authentication and pgwatch's built-in authentication:

- The reverse proxy authentication happens first
- Users must authenticate twice (proxy, then pgwatch login)
- Consider disabling pgwatch authentication if using proxy-level auth
