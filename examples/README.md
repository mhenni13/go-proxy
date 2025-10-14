# go-proxy Configuration Examples

This directory contains example configurations demonstrating various features of go-proxy.

## Kubernetes Service Discovery Example

**File:** `config-with-kubernetes-discovery.yaml`

This example demonstrates how to use Kubernetes service discovery with go-proxy. It includes:

### Features Demonstrated

1. **Basic Service Discovery** - Using Kubernetes Service ClusterIP
2. **Pod-Level Discovery** - Discovering individual pod IPs with label filtering
3. **Blue-Green Deployment** - Service discovery with version-based routing
4. **Canary Deployment** - Percentage-based traffic splitting with discovered upstreams
5. **Mixed Mode** - Combining service discovery with traditional static upstreams

### Prerequisites

To run this example, you need:

1. A running Kubernetes cluster
2. Services deployed in your cluster matching the configuration
3. Either:
   - Run go-proxy inside the cluster (uses in-cluster config)
   - Provide a kubeconfig file (for external access)

### Configuration Options

#### Service IP vs Pod IPs

```yaml
# Option 1: Use Service ClusterIP (single upstream)
use_endpoints: false

# Option 2: Discover individual pod IPs (multiple upstreams)
use_endpoints: true
```

#### Label Filtering

When using `use_endpoints: true`, you can filter pods by labels:

```yaml
labels:
  app: backend
  version: v2
  environment: production
```

#### Refresh Period

Control how often upstreams are updated:

```yaml
refresh_period: 30s   # Every 30 seconds
refresh_period: 1m    # Every minute
refresh_period: 5m    # Every 5 minutes
```

Set to `0` or omit for no automatic refresh (upstreams discovered once at startup).

### Running the Example

#### Inside Kubernetes

Deploy go-proxy as a pod in your cluster:

```bash
# The proxy will automatically use in-cluster service account
./go-proxy -config examples/config-with-kubernetes-discovery.yaml
```

#### Outside Kubernetes

Provide a kubeconfig file:

```yaml
service_discovery:
  type: kubernetes
  kubernetes:
    namespace: default
    service_name: my-service
    port: 8080
    kubeconfig: /path/to/kubeconfig
```

Then run:

```bash
./go-proxy -config examples/config-with-kubernetes-discovery.yaml
```

### RBAC Requirements

If running inside Kubernetes, ensure your service account has permissions to read Services and Endpoints:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: go-proxy
  namespace: default
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: go-proxy-discovery
  namespace: default
rules:
- apiGroups: [""]
  resources: ["services", "endpoints", "pods"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: go-proxy-discovery
  namespace: default
subjects:
- kind: ServiceAccount
  name: go-proxy
  namespace: default
roleRef:
  kind: Role
  name: go-proxy-discovery
  apiGroup: rbac.authorization.k8s.io
```

### Integration with Deployment Strategies

Service discovery works seamlessly with all deployment strategies:

#### Blue-Green

```yaml
service_discovery:
  type: kubernetes
  kubernetes:
    use_endpoints: true
deployment_strategy:
  type: blue-green
  active_version: v2.0.0
```

Pods must have `version` labels matching the `active_version`.

#### Canary

```yaml
service_discovery:
  type: kubernetes
  kubernetes:
    use_endpoints: true
deployment_strategy:
  type: canary
  canary_percent: 10
```

Requires exactly 2 versions in the discovered pods.

#### Rolling

```yaml
service_discovery:
  type: kubernetes
  kubernetes:
    use_endpoints: true
deployment_strategy:
  type: rolling
```

Pods must have `weight` labels for traffic distribution.

### Monitoring

Watch for service discovery logs:

```json
{
  "type": "service_discovery",
  "provider": "kubernetes:default/backend-service",
  "route": "/api/backend/*",
  "upstreams_count": 3
}
```

Upstreams are updated automatically based on `refresh_period`.

### Best Practices

1. **Use `use_endpoints: true`** for direct pod-to-pod communication and better load balancing
2. **Set appropriate `refresh_period`** - balance between freshness and API load
3. **Use label filtering** to target specific pod versions or groups
4. **Configure RBAC properly** when running in-cluster
5. **Set `max_retries`** to handle pod restarts gracefully
6. **Monitor logs** for discovery errors and upstream changes
7. **Test failover** by scaling pods up/down and observing traffic distribution

### Troubleshooting

**No upstreams discovered:**
- Check namespace and service name
- Verify RBAC permissions
- Ensure pods are in Ready state
- Check label filters (if using `use_endpoints: true`)

**Discovery not updating:**
- Verify `refresh_period` is set and valid
- Check proxy logs for discovery errors
- Ensure kubeconfig is valid (if used)

**Connection failures:**
- Verify TLS settings match pod configuration
- Check network policies allow traffic
- Ensure port numbers are correct
