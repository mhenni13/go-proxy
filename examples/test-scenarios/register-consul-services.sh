#!/bin/bash

# Script to register test services with Consul
# Usage: ./register-consul-services.sh [consul-address]

CONSUL_ADDR=${1:-localhost:8500}

echo "Registering services with Consul at $CONSUL_ADDR..."

# Function to register a service
register_service() {
    local service_id=$1
    local service_name=$2
    local address=$3
    local port=$4
    local version=$5
    local tags=$6

    echo "Registering $service_id ($address:$port)..."

    curl -s -X PUT "http://$CONSUL_ADDR/v1/agent/service/register" \
        -H "Content-Type: application/json" \
        -d @- << EOF
{
  "ID": "$service_id",
  "Name": "$service_name",
  "Tags": $tags,
  "Address": "$address",
  "Port": $port,
  "Meta": {
    "version": "$version"
  },
  "Check": {
    "HTTP": "http://$address:$port/",
    "Interval": "10s",
    "Timeout": "5s"
  }
}
EOF

    if [ $? -eq 0 ]; then
        echo "✓ Registered $service_id"
    else
        echo "✗ Failed to register $service_id"
    fi
}

# Register v1 backends
register_service "backend-1" "backend-service" "127.0.0.1" 9001 "v1.0.0" '["v1", "production"]'
register_service "backend-2" "backend-service" "127.0.0.1" 9002 "v1.0.0" '["v1", "production"]'
register_service "backend-3" "backend-service" "127.0.0.1" 9003 "v1.0.0" '["v1", "production"]'

# Register v2 backend (canary)
register_service "backend-v2-1" "backend-service" "127.0.0.1" 9004 "v2.0.0" '["v2", "canary"]'

echo ""
echo "Service registration complete!"
echo ""
echo "Verify services:"
echo "  curl http://$CONSUL_ADDR/v1/catalog/service/backend-service | jq ."
echo ""
echo "Check health:"
echo "  curl http://$CONSUL_ADDR/v1/health/service/backend-service?passing | jq ."
echo ""
echo "Open Consul UI:"
echo "  http://$CONSUL_ADDR/ui"
