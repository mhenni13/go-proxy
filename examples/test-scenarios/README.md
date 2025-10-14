# Service Discovery Test Scenarios

This directory contains comprehensive test scenarios and automation scripts for testing go-proxy service discovery features.

## Overview

Service discovery allows go-proxy to automatically discover and load balance across backend services without manual configuration. This enables:

- **Dynamic scaling**: Backends can be added/removed without restarting the proxy
- **Health checking**: Unhealthy backends are automatically removed from rotation
- **Zero-downtime deployments**: Blue-green, canary, and rolling deployments
- **Multi-environment support**: Different discovery mechanisms for different environments

## Available Test Scenarios

### 1. Kubernetes Service Discovery

**File**: `kubernetes-test-scenario.md`

Comprehensive guide for testing Kubernetes integration including:
- Basic Service ClusterIP discovery
- Pod-level discovery with Endpoints
- Label-based filtering
- Blue-green and canary deployments
- RBAC setup and permissions
- Multi-replica scaling tests

**Prerequisites**:
- Kubernetes cluster (minikube, kind, k3s, or cloud)
- kubectl configured
- go-proxy built

**Quick Start**:
```bash
# Follow the guide step-by-step
cat kubernetes-test-scenario.md

# Or use the example config
kubectl apply -f ../config-with-kubernetes-discovery.yaml
```

### 2. Consul Service Discovery

**File**: `consul-test-scenario.md`

Complete Consul integration testing guide covering:
- Basic service registration and discovery
- Health check monitoring and failover
- Tag-based service filtering
- Multi-datacenter discovery
- ACL token authentication
- Load balancing strategies

**Prerequisites**:
- Docker or Consul binary
- go-proxy built
- curl and jq

**Quick Start**:
```bash
# Automated setup (easiest)
./quick-start-consul.sh

# Follow interactive guide
cat consul-test-scenario.md

# Cleanup when done
./cleanup-consul-test.sh
```

## Automation Scripts

### Consul Test Automation

#### `quick-start-consul.sh`
One-command setup for Consul testing:
- Starts Consul server in Docker
- Creates 4 mock backend services (3x v1, 1x v2)
- Registers services with health checks
- Provides next-step instructions

Usage:
```bash
./quick-start-consul.sh
```

#### `register-consul-services.sh`
Manually register services with Consul:
```bash
# Default (localhost:8500)
./register-consul-services.sh

# Custom Consul address
./register-consul-services.sh consul.example.com:8500
```

#### `cleanup-consul-test.sh`
Complete cleanup of test environment:
```bash
./cleanup-consul-test.sh
```

### Docker Compose

#### `docker-compose-consul.yaml`
Full Consul test stack with Docker Compose:
- Consul server with UI
- 3 backend service instances (v1)
- 1 canary backend (v2)
- Service registrator
- Optional go-proxy container

Usage:
```bash
# Start entire stack
docker-compose -f docker-compose-consul.yaml up -d

# View logs
docker-compose -f docker-compose-consul.yaml logs -f

# Stop and clean up
docker-compose -f docker-compose-consul.yaml down -v
```

## Test Scenarios by Feature

### Basic Discovery

**Kubernetes**:
```yaml
service_discovery:
  type: kubernetes
  kubernetes:
    namespace: default
    service_name: backend
    port: 8080
    use_endpoints: false
```

**Consul**:
```yaml
service_discovery:
  type: consul
  consul:
    address: localhost:8500
    service_name: backend-service
    only_passing: true
```

### Dynamic Scaling

Test automatic upstream updates:

**Kubernetes**:
```bash
# Scale up
kubectl scale deployment backend --replicas=5

# Scale down
kubectl scale deployment backend --replicas=2
```

**Consul**:
```bash
# Add service
curl -X PUT http://localhost:8500/v1/agent/service/register -d '{...}'

# Remove service
curl -X PUT http://localhost:8500/v1/agent/service/deregister/backend-5
```

### Health Check Failover

**Test Procedure**:
1. Make requests to proxy (all backends responding)
2. Kill one backend process
3. Wait for health check period
4. Verify traffic only goes to healthy backends
5. Restart backend
6. Verify backend is back in rotation

**Kubernetes**:
```bash
# Delete a pod
kubectl delete pod backend-xxxxx-yyyyy

# Watch new pod come up
kubectl get pods -w
```

**Consul**:
```bash
# Stop a backend
pkill -f 'backend-9002.py'

# Check health
curl http://localhost:8500/v1/health/service/backend-service?passing
```

### Blue-Green Deployment

Switch all traffic between versions instantly:

```yaml
deployment_strategy:
  type: blue-green
  active_version: v1.0.0  # Switch to v2.0.0 for instant cutover
```

**Test**:
```bash
# All v1
for i in {1..10}; do curl -s http://localhost:8080/api/ | jq -r .version; done

# Update config: active_version: v2.0.0
# Restart proxy

# All v2
for i in {1..10}; do curl -s http://localhost:8080/api/ | jq -r .version; done
```

### Canary Deployment

Gradually shift traffic to new version:

```yaml
deployment_strategy:
  type: canary
  canary_percent: 10  # 10% to new version
```

**Test**:
```bash
# Count version distribution
for i in {1..100}; do
  curl -s http://localhost:8080/api/ | jq -r .version
done | sort | uniq -c

# Expected: ~90 v1.0.0, ~10 v2.0.0
```

### Label/Tag Filtering

**Kubernetes** (label-based):
```yaml
kubernetes:
  namespace: default
  service_name: backend
  use_endpoints: true
  labels:
    environment: production
    version: v2
```

**Consul** (tag-based):
```yaml
consul:
  service_name: backend-service
  tag: production  # Only services with 'production' tag
```

## Monitoring and Debugging

### Check Discovery Status

**Kubernetes**:
```bash
# Check service
kubectl get svc backend -o yaml

# Check endpoints
kubectl get endpoints backend -o yaml

# Check pod IPs
kubectl get pods -l app=backend -o wide
```

**Consul**:
```bash
# List services
curl http://localhost:8500/v1/catalog/service/backend-service | jq .

# Check health
curl http://localhost:8500/v1/health/service/backend-service | jq .

# View in UI
open http://localhost:8500/ui
```

### go-proxy Logs

Discovery events:
```json
{"type":"service_discovery","provider":"kubernetes:default/backend","upstreams_count":3}
{"type":"log","message":"[ServiceDiscovery] Updated upstreams for route /api/*: 3 -> 5"}
```

Request routing:
```json
{"type":"request","request_id":"abc","upstream":"10.244.0.5:8080","status":200,"duration_ms":12}
```

## Common Issues and Solutions

### No Upstreams Discovered

**Kubernetes**:
- Check RBAC permissions: `kubectl auth can-i get services`
- Verify service exists: `kubectl get svc backend`
- Check pod labels match service selector

**Consul**:
- Verify Consul is reachable: `curl http://localhost:8500/v1/status/leader`
- Check service is registered: `curl http://localhost:8500/v1/catalog/services`
- Verify ACL token (if ACLs enabled)

### Discovery Not Updating

- Check `refresh_period` is set in config
- Verify go-proxy has network access to discovery service
- Look for error logs in go-proxy output

### Connection Failures

- Test backend connectivity manually
- Check TLS settings match backend configuration
- Verify firewall/network policies
- Ensure health checks are passing

## Performance Testing

Load test with discovered backends:

```bash
# Install hey
go install github.com/rakyll/hey@latest

# Run load test
hey -n 10000 -c 50 http://localhost:8080/api/

# Analyze distribution across backends
```

## Best Practices

1. **Set appropriate refresh periods**
   - Kubernetes: 30s - 1m (API load)
   - Consul: 10s - 30s (event-driven)

2. **Enable health checks**
   - Always use `only_passing: true` in Consul
   - Ensure pods have proper readiness probes in Kubernetes

3. **Use label/tag filtering**
   - Separate production from staging
   - Filter by version for canary deployments

4. **Configure retries**
   - Set `max_retries: 3` for transient failures
   - Use `retry_delay_ms: 500` to avoid overwhelming failing services

5. **Monitor discovery changes**
   - Watch go-proxy logs for upstream count changes
   - Alert on sudden drops in available upstreams

## Security Considerations

**Kubernetes**:
- Use least-privilege RBAC roles
- Limit discovery to specific namespaces
- Use network policies to restrict pod communication

**Consul**:
- Enable ACLs in production
- Use service-specific tokens with read-only permissions
- Enable TLS for Consul API communication

## Next Steps

After completing these test scenarios:

1. **Production Setup**
   - Review `../config-with-kubernetes-discovery.yaml`
   - Adapt configurations for your environment
   - Set up monitoring and alerting

2. **Advanced Features**
   - Combine service discovery with WAF rules
   - Implement rate limiting per discovered service
   - Set up multi-region discovery

3. **CI/CD Integration**
   - Automate testing in CI pipelines
   - Use discovery for staging environments
   - Implement automated canary analysis

## Contributing

Found an issue or have an improvement?
- Report bugs in the main repository
- Suggest new test scenarios
- Share your production use cases

## Resources

- [Kubernetes Service Discovery Docs](https://kubernetes.io/docs/concepts/services-networking/service/)
- [Consul Service Discovery Docs](https://www.consul.io/docs/discovery)
- [go-proxy Configuration Guide](../../CLAUDE.md)
- [Example Configurations](../)
