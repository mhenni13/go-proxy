# Consul Service Discovery Test Scenario

This guide provides step-by-step instructions to test Consul service discovery with go-proxy.

## Prerequisites

- Docker or Consul binary
- go-proxy binary built
- curl and jq (for testing)

## Scenario 1: Basic Consul Service Discovery

### Step 1: Start Consul Server

**Using Docker:**

```bash
docker run -d \
  --name consul-server \
  -p 8500:8500 \
  -p 8600:8600/udp \
  -e CONSUL_BIND_INTERFACE=eth0 \
  consul:latest agent -server -ui -bootstrap-expect=1 -client=0.0.0.0
```

**Using Consul Binary:**

```bash
consul agent -dev -ui -client=0.0.0.0
```

Verify Consul is running:

```bash
curl http://localhost:8500/v1/status/leader
# Should return: "127.0.0.1:8300"

# Open UI in browser
open http://localhost:8500/ui
```

### Step 2: Register Test Services

Register multiple instances of a backend service:

```bash
# Register backend-1
curl -X PUT http://localhost:8500/v1/agent/service/register -d '{
  "ID": "backend-1",
  "Name": "backend-service",
  "Tags": ["v1", "production"],
  "Address": "127.0.0.1",
  "Port": 9001,
  "Meta": {
    "version": "v1.0.0"
  },
  "Check": {
    "HTTP": "http://127.0.0.1:9001/health",
    "Interval": "10s",
    "Timeout": "5s"
  }
}'

# Register backend-2
curl -X PUT http://localhost:8500/v1/agent/service/register -d '{
  "ID": "backend-2",
  "Name": "backend-service",
  "Tags": ["v1", "production"],
  "Address": "127.0.0.1",
  "Port": 9002,
  "Meta": {
    "version": "v1.0.0"
  },
  "Check": {
    "HTTP": "http://127.0.0.1:9002/health",
    "Interval": "10s",
    "Timeout": "5s"
  }
}'

# Register backend-3
curl -X PUT http://localhost:8500/v1/agent/service/register -d '{
  "ID": "backend-3",
  "Name": "backend-service",
  "Tags": ["v1", "production"],
  "Address": "127.0.0.1",
  "Port": 9003,
  "Meta": {
    "version": "v1.0.0"
  },
  "Check": {
    "HTTP": "http://127.0.0.1:9003/health",
    "Interval": "10s",
    "Timeout": "5s"
  }
}'
```

Verify services are registered:

```bash
curl http://localhost:8500/v1/catalog/service/backend-service | jq .
```

### Step 3: Start Mock Backend Servers

Create a simple backend server script `mock-backend.sh`:

```bash
#!/bin/bash
PORT=$1
mkdir -p /tmp/backend-$PORT

cat > /tmp/backend-$PORT/server.py << 'EOF'
import sys
from http.server import HTTPServer, BaseHTTPRequestHandler
import json

PORT = int(sys.argv[1])

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/health":
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            self.wfile.write(b'{"status":"healthy"}')
        else:
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            response = {
                "message": f"Response from backend on port {PORT}",
                "port": PORT,
                "version": "v1.0.0"
            }
            self.wfile.write(json.dumps(response).encode())

    def log_message(self, format, *args):
        pass  # Suppress logs

print(f"Starting backend server on port {PORT}")
HTTPServer(('', PORT), Handler).serve_forever()
EOF

python3 /tmp/backend-$PORT/server.py $PORT &
```

Run the backends:

```bash
chmod +x mock-backend.sh
./mock-backend.sh 9001
./mock-backend.sh 9002
./mock-backend.sh 9003

# Test backends
curl http://localhost:9001/health
curl http://localhost:9002/
```

### Step 4: Configure go-proxy

Create `consul-test-config.yaml`:

```yaml
config:
  port: 8080
  tls: false
  auth: false
  read_timeout: 15s
  write_timeout: 15s
  idle_timeout: 60s

apis:
  - name: backend-api
    protocol: http
    public: true
    routes:
      - path: /api/*
        service_discovery:
          type: consul
          refresh_period: 10s
          consul:
            address: localhost:8500
            service_name: backend-service
            only_passing: true  # Only use healthy instances
            use_tls: false
    load_balancing: round-robin
    max_retries: 3
    retry_delay_ms: 500
    fallback_response:
      status: 503
      body: '{"error": "Backend service unavailable"}'
```

### Step 5: Run go-proxy

```bash
./go-proxy -config consul-test-config.yaml
```

Expected output:
```
[Consul] Discovered 3 upstreams from backend-service
[ServiceDiscovery] Registered provider 'consul:backend-service' for route '/api/*' with 3 upstreams (refresh: 10s)
```

### Step 6: Test Service Discovery

```bash
# Make requests - should round-robin across backends
for i in {1..9}; do
  curl http://localhost:8080/api/test
  echo ""
done
```

You should see responses from different ports (9001, 9002, 9003) in rotation.

## Scenario 2: Health Check and Failover

### Step 1: Observe Healthy Services

```bash
# All 3 backends should respond
for i in {1..6}; do
  curl -s http://localhost:8080/api/ | jq -r .port
done
# Output: 9001, 9002, 9003, 9001, 9002, 9003
```

### Step 2: Simulate Backend Failure

Kill one backend:

```bash
# Find and kill backend on port 9002
pkill -f "server.py 9002"

# Wait for health check to fail (10 seconds)
sleep 12
```

Check Consul health:

```bash
curl http://localhost:8500/v1/health/service/backend-service | jq '.[] | {ID: .Service.ID, Status: .Checks[1].Status}'
```

### Step 3: Verify Failover

```bash
# Now only 2 backends should respond
for i in {1..6}; do
  curl -s http://localhost:8080/api/ | jq -r .port
done
# Output: 9001, 9003, 9001, 9003, 9001, 9003
```

Check go-proxy logs:
```
[ServiceDiscovery] Updated upstreams for route /api/*: 3 -> 2
```

### Step 4: Restore Backend

```bash
# Restart backend
./mock-backend.sh 9002

# Wait for health check to pass (10 seconds)
sleep 12

# Verify 3 backends are back
for i in {1..6}; do
  curl -s http://localhost:8080/api/ | jq -r .port
done
# Output: 9001, 9002, 9003, 9001, 9002, 9003
```

## Scenario 3: Tag-Based Filtering

### Step 1: Register Services with Different Tags

```bash
# Register v2 backend
curl -X PUT http://localhost:8500/v1/agent/service/register -d '{
  "ID": "backend-v2-1",
  "Name": "backend-service",
  "Tags": ["v2", "canary"],
  "Address": "127.0.0.1",
  "Port": 9004,
  "Meta": {
    "version": "v2.0.0"
  },
  "Check": {
    "HTTP": "http://127.0.0.1:9004/health",
    "Interval": "10s"
  }
}'

# Start the backend
cat > /tmp/backend-9004/server.py << 'EOF'
# ... (same as above but with version v2.0.0)
EOF
python3 /tmp/backend-9004/server.py 9004 &
```

### Step 2: Configure Tag Filtering

Update configuration:

```yaml
consul:
  address: localhost:8500
  service_name: backend-service
  tag: v2  # Only discover services with 'v2' tag
  only_passing: true
```

Restart go-proxy and test:

```bash
# Should only hit v2 backend on port 9004
for i in {1..5}; do
  curl -s http://localhost:8080/api/ | jq .
done
```

## Scenario 4: Multi-Datacenter Discovery

### Step 1: Configure Multiple Datacenters (Advanced)

```bash
# Start consul in dc1
docker run -d --name consul-dc1 \
  -p 8500:8500 \
  consul agent -server -bootstrap-expect=1 -datacenter=dc1 -client=0.0.0.0

# Start consul in dc2
docker run -d --name consul-dc2 \
  -p 8501:8500 \
  consul agent -server -bootstrap-expect=1 -datacenter=dc2 -client=0.0.0.0

# Join datacenters
docker exec consul-dc1 consul join -wan $(docker inspect -f '{{.NetworkSettings.IPAddress}}' consul-dc2)
```

### Step 2: Configure Datacenter in go-proxy

```yaml
consul:
  address: localhost:8500
  service_name: backend-service
  datacenter: dc2  # Discover services from dc2
  only_passing: true
```

## Scenario 5: Consul with ACL Token

### Step 1: Enable ACL in Consul

```bash
# Create Consul config with ACL
cat > /tmp/consul-config.json << 'EOF'
{
  "datacenter": "dc1",
  "acl": {
    "enabled": true,
    "default_policy": "deny",
    "tokens": {
      "master": "my-secret-token"
    }
  }
}
EOF

# Restart Consul with ACL
docker rm -f consul-server
docker run -d --name consul-server \
  -p 8500:8500 \
  -v /tmp/consul-config.json:/consul/config/config.json \
  consul agent -server -bootstrap-expect=1 -client=0.0.0.0 -config-file=/consul/config/config.json
```

### Step 2: Create Service Read Token

```bash
# Bootstrap ACL
export CONSUL_HTTP_TOKEN=my-secret-token

# Create policy for service discovery
curl -X PUT http://localhost:8500/v1/acl/policy \
  -H "X-Consul-Token: my-secret-token" \
  -d '{
  "Name": "service-reader",
  "Description": "Read-only access to services",
  "Rules": "service_prefix \"\" { policy = \"read\" }"
}'

# Create token with this policy
curl -X PUT http://localhost:8500/v1/acl/token \
  -H "X-Consul-Token: my-secret-token" \
  -d '{
  "Description": "Service discovery token",
  "Policies": [{"Name": "service-reader"}]
}' | jq -r .SecretID
# Output: <your-token>
```

### Step 3: Configure go-proxy with Token

```yaml
consul:
  address: localhost:8500
  service_name: backend-service
  token: "<your-token>"  # Add the token here
  only_passing: true
```

## Scenario 6: Load Balancing Strategies

### Test Random Load Balancing

```yaml
load_balancing: random
```

```bash
# Distribution will be random
for i in {1..20}; do
  curl -s http://localhost:8080/api/ | jq -r .port
done | sort | uniq -c
```

### Test with Deployment Strategies

Blue-Green with Consul:

```yaml
routes:
  - path: /api/*
    service_discovery:
      type: consul
      refresh_period: 10s
      consul:
        address: localhost:8500
        service_name: backend-service
    deployment_strategy:
      type: blue-green
      active_version: v1.0.0  # Use v1 backends
```

## Monitoring and Debugging

### Check Consul Catalog

```bash
# List all services
curl http://localhost:8500/v1/catalog/services | jq .

# Get service details
curl http://localhost:8500/v1/catalog/service/backend-service | jq .

# Check service health
curl http://localhost:8500/v1/health/service/backend-service?passing=true | jq .
```

### Monitor go-proxy Discovery

```bash
# Watch logs for discovery events
tail -f go-proxy.log | grep -i consul

# Expected output:
# [Consul] Discovered 3 upstreams from backend-service
# [ServiceDiscovery] Updated upstreams for route /api/*: 3 -> 2
```

### Test Discovery API

```bash
# Query Consul directly
curl "http://localhost:8500/v1/health/service/backend-service?passing=true" | jq -r '.[] | "\(.Service.Address):\(.Service.Port)"'
```

## Troubleshooting

### No Services Discovered

```bash
# Check Consul connectivity
curl http://localhost:8500/v1/status/leader

# Verify service exists
curl http://localhost:8500/v1/catalog/service/backend-service | jq .

# Check ACL token (if enabled)
curl -H "X-Consul-Token: your-token" http://localhost:8500/v1/catalog/service/backend-service
```

### Health Checks Failing

```bash
# Check health check status
curl http://localhost:8500/v1/health/checks/backend-service | jq .

# Test health endpoint manually
curl http://localhost:9001/health

# View detailed service info
consul catalog nodes -service=backend-service -detailed
```

### Connection Refused

```bash
# Verify backends are listening
netstat -tlnp | grep -E '900[1-4]'

# Test backend directly
curl http://localhost:9001/

# Check go-proxy can reach backends
telnet localhost 9001
```

## Load Testing

```bash
# Install hey (HTTP load generator)
go install github.com/rakyll/hey@latest

# Run load test
hey -n 10000 -c 50 http://localhost:8080/api/

# Results will show distribution across backends
```

## Cleanup

```bash
# Stop backends
pkill -f "server.py"

# Deregister services
curl -X PUT http://localhost:8500/v1/agent/service/deregister/backend-1
curl -X PUT http://localhost:8500/v1/agent/service/deregister/backend-2
curl -X PUT http://localhost:8500/v1/agent/service/deregister/backend-3
curl -X PUT http://localhost:8500/v1/agent/service/deregister/backend-v2-1

# Stop Consul
docker rm -f consul-server

# Or if using binary
pkill consul
```

## Advanced: Service Mesh Integration

For production environments, consider integrating with Consul Connect for mTLS:

```yaml
consul:
  address: localhost:8500
  service_name: backend-service
  use_tls: true  # Enable TLS for discovered upstreams
  only_passing: true
```

This enables secure service-to-service communication in a Consul service mesh.

## Expected Metrics

Successful discovery:
- Initial discovery: 3 upstreams
- After health check failure: 2 upstreams
- After recovery: 3 upstreams
- Response time: < 20ms
- Load distribution: Even across healthy backends
