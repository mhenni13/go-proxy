# Go Proxy

A configurable HTTP/HTTPS reverse proxy with JWT and Basic authentication, rate limiting, load balancing, CORS, and fallback responses. Supports Vulcand-style predicate-based access control for JWT claims.  

---

## Features

- Reverse proxy with HTTP/HTTPS support  
- JWT and Basic authentication with group-based permissions  
- Rate limiting per API  
- Load balancing among multiple upstream servers  
- Custom headers, cookies, and CORS support  
- Fallback responses (including optional Base64 body)  
- Optional TLS for upstream servers  
- Predicate-based authorization for JWTs  

---

## Configuration

The proxy is configured using a YAML file.

### Command Line Options

```bash
# Basic usage
./go-proxy -c config.yaml

# With WAF logging
./go-proxy -c config.yaml -l waf.log

# With IP list file (allowlist or blocklist)
./go-proxy -c config.yaml -s list.black

# Combined
./go-proxy -c config.yaml -l waf.log -s list.white
```

**Available Flags:**
- `-c`, `--config`: Path to configuration file (default: `internal/config/config.yaml`)
- `-l`, `--log`: Path to WAF log file (overrides config file setting)
- `-s`, `--security-list`: Path to IP list file - allowlist or blocklist depending on mode (overrides config file setting)

### Example `config.yaml`

```yaml
config:
  port: 8080
  tls: false
  tls_cert_file: "/path/to/cert.pem"  # Required when tls: true
  tls_key_file: "/path/to/key.pem"    # Required when tls: true
  auth: true
  read_timeout: "15s"
  write_timeout: "15s"
  idle_timeout: "60s"

auth:
  - group: admins
    type: jwt
    jwt_secret: "supersecret"
    verify: 'have("admin") || have("superadmin")'

  - group: devops
    type: jwt
    jwt_secret: "devopssecret"
    verify: 'is("group","devops")'

  - group: users
    type: basic
    users:
      - username: alice
        password_hash: "$2a$10$e0NRPmJY05KmY/tP7o3f5e7Zjw0n4M8xYx8oBhrx/4eO8a5XNUNdK"

apis:
  - name: sales_api
    protocol: http
    permissions: 
      - admins
    max_retries: 3
    retry_delay_ms: 100
    rate_limit: 100
    load_balancing: round-robin
    sticky_sessions: false
    routes:
      - path: /v1/sales
        upstreams:
          - host: localhost
            port: 8879
            tls: true
            tls_insecure: false
    headers:
      X-Custom-Header: "MyHeader"
    cookies:
      session: "abc123"
    cors:
      allowed_origins:
        - "*"
      allowed_methods:
        - GET
        - POST
      allowed_headers:
        - Content-Type
    fallback_response:
      status: 503
      body: '{"error":"service unavailable"}'

---

## Deployment Strategies

The proxy supports multiple deployment strategies for managing traffic between different versions of upstream services:

### Blue-Green Deployment
Routes all traffic to a single active version. Switch versions instantly by updating `active_version`.

```yaml
routes:
  - path: /api/v1
    upstreams:
      - host: localhost
        port: 8080
        version: "blue"
      - host: localhost
        port: 8081
        version: "green"
    deployment_strategy:
      type: blue-green
      active_version: "blue"  # All traffic goes to blue
```

### Canary Deployment
Gradually shift traffic to a new version using percentage-based routing.

```yaml
routes:
  - path: /api/v1
    upstreams:
      - host: localhost
        port: 8080
        version: "stable"
      - host: localhost
        port: 8081
        version: "canary"
    deployment_strategy:
      type: canary
      
      : 10  # 10% traffic to canary, 90% to stable
```

### Rolling Deployment
Distribute traffic based on weights assigned to each upstream.

```yaml
routes:
  - path: /api/v1
    upstreams:
      - host: localhost
        port: 8080
        weight: 80  # 80% of traffic
      - host: localhost
        port: 8081
        weight: 20  # 20% of traffic
    deployment_strategy:
      type: rolling
```

### Recreate Deployment
Route traffic only to the active version (old version is shut down).

```yaml
routes:
  - path: /api/v1
    upstreams:
      - host: localhost
        port: 8080
        version: "v1"
      - host: localhost
        port: 8081
        version: "v2"
    deployment_strategy:
      type: recreate
      active_version: "v2"  # Only v2 receives traffic
```

---

## Security Features

### IP Blocker
Control access based on client IP addresses using allowlists or blocklists.

**Inline Configuration:**
```yaml
security:
  ip_blocker:
    enabled: true
    mode: "blocklist"  # or "allowlist"
    allowlist:
      - "192.168.1.0/24"
      - "10.0.0.5"
    blocklist:
      - "192.168.1.100"
      - "10.0.0.0/8"
```

**File-Based Configuration:**
```yaml
security:
  ip_blocker:
    enabled: true
    mode: "blocklist"
    blocklist_file: "/path/to/blocklist.txt"  # One IP/CIDR per line
```

**IP List File Format:**
```
# Comments start with #
192.168.1.100
10.0.0.0/8
172.16.0.0/12

# Empty lines are ignored
203.0.113.0/24
```

**Modes:**
- `allowlist`: Only IPs in the allowlist can access the API
- `blocklist`: IPs in the blocklist are denied access
- `off`: IP blocking is disabled

Supports both single IPs and CIDR notation. Automatically handles `X-Forwarded-For` and `X-Real-IP` headers.

### Web Application Firewall (WAF)
Protect against common web attacks with built-in and custom rules.

```yaml
security:
  waf:
    enabled: true
    mode: "block"  # or "detect" for logging only
    log_file: "/var/log/waf/waf.log"  # Optional: dedicated WAF log file
    custom_rules:
      - id: "CUSTOM-001"
        description: "Block admin path access"
        pattern: "/(admin|root|backup)"
        target: "uri"
        action: "block"
      - id: "CUSTOM-002"
        description: "Detect sensitive data in query"
        pattern: "(password|token|secret)"
        target: "query"
        action: "log"
```

WAF events are logged both to stdout (structured JSON) and optionally to a dedicated log file specified in `log_file`.

**Built-in Protection Against:**
- SQL Injection
- Cross-Site Scripting (XSS)
- Path Traversal
- Command Injection
- Remote File Inclusion (RFI)
- LDAP Injection
- XML External Entity (XXE)
- Server-Side Request Forgery (SSRF)
- HTTP Header Injection

**WAF Modes:**
- `block`: Block requests that match rules
- `detect`: Log suspicious requests without blocking

**Rule Targets:**
- `uri`: Match against request path
- `query`: Match against query parameters
- `body`: Match against request body
- `headers`: Match against HTTP headers
- `all`: Match against all of the above

## TO DO :
- Circuit Breaking 
- Metrics and Monitoring
- Connection Pooling & Keep-Alive 
- Request/Response Transformation
- Caching Layer
- Advanced Rate Limiting (per-client/IP not only per API)
- Configuration Hot Reload 
- Service Discovery (Integration with Consul/etcd/Kubernetes service discovery)
- Request Queuing
- WebSocket Sticky Sessions
