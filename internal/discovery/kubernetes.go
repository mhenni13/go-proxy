package discovery

import (
	"context"
	"fmt"
	"log"

	"github.com/mhenni13/go-proxy/internal/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// KubernetesProvider implements Kubernetes-based service discovery
type KubernetesProvider struct {
	clientset     *kubernetes.Clientset
	namespace     string
	serviceName   string
	port          int32
	useTLS        bool
	tlsInsecure   bool
	labels        map[string]string // Additional labels for filtering endpoints
	useEndpoints  bool              // If true, discover from Endpoints, otherwise use Service
}

// KubernetesConfig holds configuration for Kubernetes service discovery
type KubernetesConfig struct {
	Namespace    string            `yaml:"namespace"`
	ServiceName  string            `yaml:"service_name"`
	Port         int32             `yaml:"port"`
	UseTLS       bool              `yaml:"use_tls"`
	TLSInsecure  bool              `yaml:"tls_insecure"`
	Labels       map[string]string `yaml:"labels,omitempty"`
	UseEndpoints bool              `yaml:"use_endpoints"` // Discover individual pod IPs
	KubeConfig   string            `yaml:"kubeconfig,omitempty"` // Path to kubeconfig file (empty = in-cluster)
}

// NewKubernetesProvider creates a new Kubernetes service discovery provider
func NewKubernetesProvider(cfg KubernetesConfig) (*KubernetesProvider, error) {
	var k8sConfig *rest.Config
	var err error

	// Use in-cluster config if no kubeconfig path provided
	if cfg.KubeConfig == "" {
		k8sConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
		}
		log.Printf("[Kubernetes] Using in-cluster configuration")
	} else {
		k8sConfig, err = clientcmd.BuildConfigFromFlags("", cfg.KubeConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to build config from kubeconfig: %w", err)
		}
		log.Printf("[Kubernetes] Using kubeconfig from: %s", cfg.KubeConfig)
	}

	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return &KubernetesProvider{
		clientset:    clientset,
		namespace:    cfg.Namespace,
		serviceName:  cfg.ServiceName,
		port:         cfg.Port,
		useTLS:       cfg.UseTLS,
		tlsInsecure:  cfg.TLSInsecure,
		labels:       cfg.Labels,
		useEndpoints: cfg.UseEndpoints,
	}, nil
}

// Discover returns upstreams from Kubernetes service/endpoints
func (p *KubernetesProvider) Discover(ctx context.Context) ([]config.Upstream, error) {
	if p.useEndpoints {
		return p.discoverFromEndpoints(ctx)
	}
	return p.discoverFromService(ctx)
}

// discoverFromService discovers upstreams from a Kubernetes Service (uses Service IP)
func (p *KubernetesProvider) discoverFromService(ctx context.Context) ([]config.Upstream, error) {
	service, err := p.clientset.CoreV1().Services(p.namespace).Get(ctx, p.serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get service %s/%s: %w", p.namespace, p.serviceName, err)
	}

	if service.Spec.ClusterIP == "" || service.Spec.ClusterIP == "None" {
		return nil, fmt.Errorf("service %s/%s has no ClusterIP (headless service?)", p.namespace, p.serviceName)
	}

	// Find the target port
	var targetPort int32
	if p.port > 0 {
		targetPort = p.port
	} else if len(service.Spec.Ports) > 0 {
		targetPort = service.Spec.Ports[0].Port
	} else {
		return nil, fmt.Errorf("no ports defined for service %s/%s", p.namespace, p.serviceName)
	}

	useTLS := p.useTLS
	tlsInsecure := p.tlsInsecure

	upstream := config.Upstream{
		Host:        service.Spec.ClusterIP,
		Port:        int(targetPort),
		TLS:         &useTLS,
		TLSInsecure: &tlsInsecure,
	}

	return []config.Upstream{upstream}, nil
}

// discoverFromEndpoints discovers upstreams from Kubernetes Endpoints (uses pod IPs)
func (p *KubernetesProvider) discoverFromEndpoints(ctx context.Context) ([]config.Upstream, error) {
	endpoints, err := p.clientset.CoreV1().Endpoints(p.namespace).Get(ctx, p.serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoints %s/%s: %w", p.namespace, p.serviceName, err)
	}

	var upstreams []config.Upstream
	useTLS := p.useTLS
	tlsInsecure := p.tlsInsecure

	for _, subset := range endpoints.Subsets {
		// Find the target port
		var targetPort int32
		if p.port > 0 {
			targetPort = p.port
		} else if len(subset.Ports) > 0 {
			targetPort = subset.Ports[0].Port
		} else {
			continue
		}

		// Add all ready addresses
		for _, addr := range subset.Addresses {
			// Filter by labels if provided
			if len(p.labels) > 0 && !p.matchesLabels(addr.TargetRef) {
				continue
			}

			upstream := config.Upstream{
				Host:        addr.IP,
				Port:        int(targetPort),
				TLS:         &useTLS,
				TLSInsecure: &tlsInsecure,
			}
			upstreams = append(upstreams, upstream)
		}
	}

	if len(upstreams) == 0 {
		return nil, fmt.Errorf("no ready endpoints found for service %s/%s", p.namespace, p.serviceName)
	}

	return upstreams, nil
}

// matchesLabels checks if a pod matches the required labels
func (p *KubernetesProvider) matchesLabels(targetRef *corev1.ObjectReference) bool {
	if targetRef == nil || targetRef.Kind != "Pod" {
		return true // If no target ref or not a pod, allow it
	}

	// Fetch pod to check labels
	pod, err := p.clientset.CoreV1().Pods(targetRef.Namespace).Get(context.Background(), targetRef.Name, metav1.GetOptions{})
	if err != nil {
		log.Printf("[Kubernetes] Failed to get pod %s/%s: %v", targetRef.Namespace, targetRef.Name, err)
		return false
	}

	// Check if pod labels match
	for key, value := range p.labels {
		if podValue, exists := pod.Labels[key]; !exists || podValue != value {
			return false
		}
	}

	return true
}

// Name returns the provider name
func (p *KubernetesProvider) Name() string {
	return fmt.Sprintf("kubernetes:%s/%s", p.namespace, p.serviceName)
}

// Close releases resources
func (p *KubernetesProvider) Close() error {
	// Kubernetes client doesn't require explicit cleanup
	return nil
}
