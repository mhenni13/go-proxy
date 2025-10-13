package proxy

import (
	"math/rand"
	"sync"
	"github.com/mhenni13/go-proxy/internal/config"
)

type LoadBalancer struct {
	strategy  string
	upstreams []config.Upstream
	index     int
	mu        sync.Mutex
}

func NewLoadBalancer(strategy string, upstreams []config.Upstream) *LoadBalancer {
	return &LoadBalancer{
		strategy:  strategy,
		upstreams: upstreams,
		index:     0,
	}
}

func (lb *LoadBalancer) Next() config.Upstream {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	if lb.strategy == "random" {
		return lb.upstreams[rand.Intn(len(lb.upstreams))]
	}
	// round_robin default
	up := lb.upstreams[lb.index]
	lb.index = (lb.index + 1) % len(lb.upstreams)
	return up
}
