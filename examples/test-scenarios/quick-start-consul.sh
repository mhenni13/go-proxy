#!/bin/bash

# Quick start script for testing Consul service discovery
# This script sets up everything needed for Consul discovery testing

set -e

echo "=== Consul Service Discovery Quick Start ==="
echo ""

# Check prerequisites
command -v docker >/dev/null 2>&1 || { echo "Error: docker is required but not installed."; exit 1; }
command -v curl >/dev/null 2>&1 || { echo "Error: curl is required but not installed."; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "Warning: jq is not installed. Install it for better output formatting."; }

# Start Consul
echo "Step 1: Starting Consul server..."
docker run -d \
  --name consul-test \
  -p 8500:8500 \
  -e CONSUL_BIND_INTERFACE=eth0 \
  consul:latest agent -server -ui -bootstrap-expect=1 -client=0.0.0.0

echo "Waiting for Consul to be ready..."
sleep 5

# Check Consul
if curl -s http://localhost:8500/v1/status/leader > /dev/null; then
    echo "✓ Consul is running"
else
    echo "✗ Consul failed to start"
    exit 1
fi

# Create backend servers directory
mkdir -p /tmp/consul-test-backends

# Start mock backends
echo ""
echo "Step 2: Starting backend services..."

for PORT in 9001 9002 9003 9004; do
    VERSION="v1.0.0"
    if [ $PORT -eq 9004 ]; then
        VERSION="v2.0.0"
    fi

    cat > /tmp/consul-test-backends/backend-$PORT.py << EOF
import sys
from http.server import HTTPServer, BaseHTTPRequestHandler
import json
import socket

PORT = $PORT
VERSION = "$VERSION"

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-type', 'application/json')
        self.send_header('X-Backend-Port', str(PORT))
        self.end_headers()

        response = {
            "message": f"Response from backend on port {PORT}",
            "port": PORT,
            "version": VERSION,
            "hostname": socket.gethostname()
        }
        self.wfile.write(json.dumps(response, indent=2).encode())

    def log_message(self, format, *args):
        pass  # Suppress logs

print(f"Backend {PORT} (version {VERSION}) started")
HTTPServer(('0.0.0.0', PORT), Handler).serve_forever()
EOF

    python3 /tmp/consul-test-backends/backend-$PORT.py &
    BACKEND_PID=$!
    echo "✓ Started backend on port $PORT (PID: $BACKEND_PID)"
done

sleep 2

# Register services with Consul
echo ""
echo "Step 3: Registering services with Consul..."

# Register v1 backends
for i in 1 2 3; do
    PORT=$((9000 + i))
    curl -s -X PUT http://localhost:8500/v1/agent/service/register -d @- > /dev/null << EOF
{
  "ID": "backend-$i",
  "Name": "backend-service",
  "Tags": ["v1", "production"],
  "Address": "127.0.0.1",
  "Port": $PORT,
  "Meta": {
    "version": "v1.0.0"
  },
  "Check": {
    "HTTP": "http://127.0.0.1:$PORT/",
    "Interval": "10s",
    "Timeout": "5s"
  }
}
EOF
    echo "✓ Registered backend-$i"
done

# Register v2 backend
curl -s -X PUT http://localhost:8500/v1/agent/service/register -d @- > /dev/null << 'EOF'
{
  "ID": "backend-v2-1",
  "Name": "backend-service",
  "Tags": ["v2", "canary"],
  "Address": "127.0.0.1",
  "Port": 9004,
  "Meta": {
    "version": "v2.0.0"
  },
  "Check": {
    "HTTP": "http://127.0.0.1:9004/",
    "Interval": "10s",
    "Timeout": "5s"
  }
}
EOF
echo "✓ Registered backend-v2-1"

sleep 2

# Verify
echo ""
echo "Step 4: Verifying setup..."
SERVICE_COUNT=$(curl -s http://localhost:8500/v1/catalog/service/backend-service | jq '. | length')
echo "✓ $SERVICE_COUNT services registered"

# Test backends
echo ""
echo "Step 5: Testing backends..."
for PORT in 9001 9002 9003 9004; do
    if curl -s http://localhost:$PORT/ > /dev/null; then
        echo "✓ Backend on port $PORT is responding"
    else
        echo "✗ Backend on port $PORT is not responding"
    fi
done

echo ""
echo "=== Setup Complete! ==="
echo ""
echo "Next steps:"
echo ""
echo "1. Create go-proxy config (consul-test-config.yaml):"
cat << 'YAML'
---
config:
  port: 8080
  tls: false
  auth: false

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
            only_passing: true
    load_balancing: round-robin
    max_retries: 3
    retry_delay_ms: 500
    fallback_response:
      status: 503
      body: '{"error": "Service unavailable"}'
YAML
echo ""
echo "2. Run go-proxy:"
echo "   ./go-proxy -config consul-test-config.yaml"
echo ""
echo "3. Test the proxy:"
echo "   for i in {1..10}; do curl http://localhost:8080/api/; echo; done"
echo ""
echo "4. Monitor Consul UI:"
echo "   http://localhost:8500/ui"
echo ""
echo "5. Test failover by stopping a backend:"
echo "   pkill -f 'backend-9002.py'"
echo "   # Wait 10 seconds, then test again"
echo ""
echo "Cleanup:"
echo "   ./cleanup-consul-test.sh"
echo ""

# Save PIDs for cleanup
ps aux | grep 'backend-900[1-4].py' | grep -v grep | awk '{print $2}' > /tmp/consul-test-backends/pids.txt
echo "Backend PIDs saved to /tmp/consul-test-backends/pids.txt"
