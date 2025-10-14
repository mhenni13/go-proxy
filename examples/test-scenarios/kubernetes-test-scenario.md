# Kubernetes Service Discovery Test Scenario

This guide provides step-by-step instructions to test Kubernetes service discovery with go-proxy.

## Prerequisites

- Kubernetes cluster (minikube, kind, or cloud provider)
- kubectl configured
- go-proxy binary built

## Scenario 1: Basic Service Discovery with ClusterIP

### Step 1: Deploy a Test Backend Application

Create a simple backend deployment:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-backend
  namespace: default
spec:
  replicas: 3
  selector:
    matchLabels:
      app: test-backend
  template:
    metadata:
      labels:
        app: test-backend
        version: v1.0.0
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
        ports:
        - containerPort: 80
        env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        volumeMounts:
        - name: html
          mountPath: /usr/share/nginx/html
      initContainers:
      - name: setup
        image: busybox
        command:
        - sh
        - -c
        - |
          echo "<h1>Backend Pod: \$HOSTNAME</h1>" > /html/index.html
          echo "<p>Version: v1.0.0</p>" >> /html/index.html
        volumeMounts:
        - name: html
          mountPath: /html
      volumes:
      - name: html
        emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: test-backend
  namespace: default
spec:
  selector:
    app: test-backend
  ports:
  - protocol: TCP
    port: 80
    targetPort: 80
EOF
```

Verify the pods are running:

```bash
kubectl get pods -l app=test-backend
kubectl get svc test-backend
```

### Step 2: Create RBAC for go-proxy

```bash
cat <<EOF | kubectl apply -f -
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
EOF
```

### Step 3: Configure go-proxy

Create `kubernetes-test-config.yaml`:

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
          type: kubernetes
          refresh_period: 10s
          kubernetes:
            namespace: default
            service_name: test-backend
            port: 80
            use_tls: false
            use_endpoints: false  # Use Service ClusterIP
    load_balancing: round-robin
    max_retries: 3
    retry_delay_ms: 500
    fallback_response:
      status: 503
      body: '{"error": "Backend service unavailable"}'
```

### Step 4: Run go-proxy (Two Options)

**Option A: Run Outside Cluster (Requires kubeconfig)**

```bash
# Add kubeconfig path to configuration
sed -i 's/use_endpoints: false/use_endpoints: false\n            kubeconfig: ~/.kube\/config/' kubernetes-test-config.yaml

# Run go-proxy
./go-proxy -config kubernetes-test-config.yaml
```

**Option B: Run Inside Cluster**

Create deployment:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: go-proxy-config
  namespace: default
data:
  config.yaml: |
    $(cat kubernetes-test-config.yaml | sed 's/^/    /')
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: go-proxy
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: go-proxy
  template:
    metadata:
      labels:
        app: go-proxy
    spec:
      serviceAccountName: go-proxy
      containers:
      - name: go-proxy
        image: your-registry/go-proxy:latest  # Build and push your image
        ports:
        - containerPort: 8080
        volumeMounts:
        - name: config
          mountPath: /etc/go-proxy
        command: ["/go-proxy", "-config", "/etc/go-proxy/config.yaml"]
      volumes:
      - name: config
        configMap:
          name: go-proxy-config
---
apiVersion: v1
kind: Service
metadata:
  name: go-proxy
  namespace: default
spec:
  selector:
    app: go-proxy
  ports:
  - protocol: TCP
    port: 8080
    targetPort: 8080
  type: LoadBalancer  # or NodePort for local testing
EOF
```

### Step 5: Test the Discovery

```bash
# If running outside cluster
curl http://localhost:8080/api/

# If running inside cluster
kubectl port-forward svc/go-proxy 8080:8080
curl http://localhost:8080/api/

# You should see responses from the backend pods
```

### Step 6: Verify Discovery is Working

Watch the logs:

```bash
# Outside cluster
tail -f go-proxy.log

# Inside cluster
kubectl logs -f deployment/go-proxy
```

You should see logs like:
```json
{"type":"service_discovery","provider":"kubernetes:default/test-backend","upstreams_count":1}
```

## Scenario 2: Pod-Level Discovery with Endpoints

### Step 1: Update Configuration for Endpoints Discovery

Modify `kubernetes-test-config.yaml`:

```yaml
service_discovery:
  type: kubernetes
  refresh_period: 10s
  kubernetes:
    namespace: default
    service_name: test-backend
    port: 80
    use_tls: false
    use_endpoints: true  # Discover individual pod IPs
```

Restart go-proxy and test again. Now go-proxy will load balance across individual pod IPs.

### Step 2: Test Load Balancing

Make multiple requests and observe different pod responses:

```bash
for i in {1..10}; do
  curl http://localhost:8080/api/
  echo ""
done
```

You should see responses from different backend pods (test-backend-xxxx-yyyy).

### Step 3: Test Dynamic Updates

Scale the deployment:

```bash
# Scale up
kubectl scale deployment test-backend --replicas=5

# Wait 10 seconds (refresh period)
sleep 10

# Make requests - you should see 5 different pods
for i in {1..10}; do curl -s http://localhost:8080/api/ | grep "Pod:"; done

# Scale down
kubectl scale deployment test-backend --replicas=1

# Wait 10 seconds
sleep 10

# Make requests - you should see only 1 pod
for i in {1..5}; do curl -s http://localhost:8080/api/ | grep "Pod:"; done
```

## Scenario 3: Label-Based Filtering

### Step 1: Deploy Multiple Versions

```bash
# Deploy v2.0.0
cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-backend-v2
  namespace: default
spec:
  replicas: 2
  selector:
    matchLabels:
      app: test-backend
      version: v2.0.0
  template:
    metadata:
      labels:
        app: test-backend
        version: v2.0.0
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
        ports:
        - containerPort: 80
        volumeMounts:
        - name: html
          mountPath: /usr/share/nginx/html
      initContainers:
      - name: setup
        image: busybox
        command:
        - sh
        - -c
        - |
          echo "<h1>Backend Pod: \$HOSTNAME</h1>" > /html/index.html
          echo "<p>Version: v2.0.0 (NEW!)</p>" >> /html/index.html
        volumeMounts:
        - name: html
          mountPath: /html
      volumes:
      - name: html
        emptyDir: {}
EOF
```

### Step 2: Configure Label Filtering

Update configuration to only discover v2 pods:

```yaml
service_discovery:
  type: kubernetes
  refresh_period: 10s
  kubernetes:
    namespace: default
    service_name: test-backend
    port: 80
    use_tls: false
    use_endpoints: true
    labels:
      app: test-backend
      version: v2.0.0  # Only discover v2 pods
```

### Step 3: Test Filtering

```bash
# Restart go-proxy with new config
# Make requests - you should only see v2.0.0 responses
for i in {1..10}; do curl -s http://localhost:8080/api/ | grep "Version:"; done
```

## Scenario 4: Blue-Green Deployment with Discovery

### Step 1: Configure Blue-Green Strategy

```yaml
routes:
  - path: /api/*
    service_discovery:
      type: kubernetes
      refresh_period: 10s
      kubernetes:
        namespace: default
        service_name: test-backend
        port: 80
        use_endpoints: true
    deployment_strategy:
      type: blue-green
      active_version: v1.0.0  # All traffic to v1
```

### Step 2: Test Version Switch

```bash
# All traffic goes to v1
for i in {1..5}; do curl -s http://localhost:8080/api/ | grep "Version:"; done

# Switch to v2 by updating config and reloading
# Change active_version: v2.0.0
# Restart go-proxy

# All traffic now goes to v2
for i in {1..5}; do curl -s http://localhost:8080/api/ | grep "Version:"; done
```

## Scenario 5: Canary Deployment

### Step 1: Configure Canary

```yaml
deployment_strategy:
  type: canary
  canary_percent: 20  # 20% traffic to v2, 80% to v1
```

### Step 2: Test Traffic Split

```bash
# Make 100 requests and count versions
for i in {1..100}; do
  curl -s http://localhost:8080/api/ | grep -o "v[0-9].[0-9].[0-9]"
done | sort | uniq -c

# Expected output (approximately):
# 80 v1.0.0
# 20 v2.0.0
```

## Troubleshooting

### Discovery Not Working

```bash
# Check RBAC permissions
kubectl auth can-i get services --as=system:serviceaccount:default:go-proxy
kubectl auth can-i get endpoints --as=system:serviceaccount:default:go-proxy
kubectl auth can-i list pods --as=system:serviceaccount:default:go-proxy

# Check service and endpoints exist
kubectl get svc test-backend
kubectl get endpoints test-backend

# Check go-proxy logs
kubectl logs -f deployment/go-proxy | grep -i discovery
```

### No Upstreams Discovered

```bash
# Verify pods are ready
kubectl get pods -l app=test-backend -o wide

# Check service selector matches pod labels
kubectl describe svc test-backend
kubectl get pods -l app=test-backend --show-labels

# Test discovery manually
kubectl get endpoints test-backend -o yaml
```

### Connection Failures

```bash
# Test connectivity from go-proxy pod to backend
kubectl exec -it deployment/go-proxy -- wget -O- http://test-backend.default.svc.cluster.local

# Check network policies
kubectl get networkpolicies

# Verify DNS resolution
kubectl exec -it deployment/go-proxy -- nslookup test-backend.default.svc.cluster.local
```

## Expected Log Output

Successful discovery:
```json
{"type":"log","level":"info","message":"[ServiceDiscovery] Registered provider 'kubernetes:default/test-backend' for route '/api/*' with 3 upstreams (refresh: 10s)"}
{"type":"service_discovery","provider":"kubernetes:default/test-backend","upstreams_count":3}
{"type":"log","level":"info","message":"[ServiceDiscovery] Updated upstreams for route /api/*: 3 -> 5"}
```

Request flow:
```json
{"type":"request","request_id":"abc123","method":"GET","path":"/api/","upstream":"10.244.0.5:80","status":200,"duration_ms":12}
```

## Cleanup

```bash
kubectl delete deployment test-backend test-backend-v2 go-proxy
kubectl delete svc test-backend go-proxy
kubectl delete sa go-proxy
kubectl delete role go-proxy-discovery
kubectl delete rolebinding go-proxy-discovery
kubectl delete configmap go-proxy-config
```
