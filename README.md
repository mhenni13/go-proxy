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

### Example `config.yaml`

```yaml
config:
  port: 8080
  tls: false
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
