package deployment

import (
	"fmt"
	"math/rand"
	"sync"

	"github.com/mhenni13/go-proxy/internal/config"
)

// Strategy defines the interface for deployment strategies
type Strategy interface {
	SelectUpstream() (config.Upstream, error)
	UpdateUpstreams(upstreams []config.Upstream)
}

// NewStrategy creates a deployment strategy based on config
func NewStrategy(deploymentConfig config.DeploymentConfig, upstreams []config.Upstream, loadBalancing string) (Strategy, error) {
	if deploymentConfig.Type == "" {
		// No deployment strategy specified, use standard load balancing
		return &StandardStrategy{
			upstreams:     upstreams,
			loadBalancing: loadBalancing,
		}, nil
	}

	switch deploymentConfig.Type {
	case "blue-green":
		return NewBlueGreenStrategy(deploymentConfig, upstreams)
	case "canary":
		return NewCanaryStrategy(deploymentConfig, upstreams)
	case "rolling":
		return NewRollingStrategy(deploymentConfig, upstreams)
	case "recreate":
		return NewRecreateStrategy(deploymentConfig, upstreams)
	default:
		return nil, fmt.Errorf("unknown deployment strategy: %s", deploymentConfig.Type)
	}
}

// StandardStrategy uses traditional load balancing (round-robin or random)
type StandardStrategy struct {
	upstreams     []config.Upstream
	loadBalancing string
	index         int
	mu            sync.Mutex
}

func (s *StandardStrategy) SelectUpstream() (config.Upstream, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.upstreams) == 0 {
		return config.Upstream{}, fmt.Errorf("no upstreams available")
	}

	if s.loadBalancing == "random" {
		return s.upstreams[rand.Intn(len(s.upstreams))], nil
	}

	// round-robin default
	up := s.upstreams[s.index]
	s.index = (s.index + 1) % len(s.upstreams)
	return up, nil
}

func (s *StandardStrategy) UpdateUpstreams(upstreams []config.Upstream) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upstreams = upstreams
	if s.index >= len(upstreams) {
		s.index = 0
	}
}

// BlueGreenStrategy routes all traffic to the active version
type BlueGreenStrategy struct {
	activeVersion string
	upstreams     map[string][]config.Upstream // version -> upstreams
	mu            sync.RWMutex
}

func NewBlueGreenStrategy(cfg config.DeploymentConfig, upstreams []config.Upstream) (*BlueGreenStrategy, error) {
	if cfg.ActiveVersion == "" {
		return nil, fmt.Errorf("active_version is required for blue-green deployment")
	}

	// Group upstreams by version
	versionMap := make(map[string][]config.Upstream)
	for _, up := range upstreams {
		if up.Version == "" {
			return nil, fmt.Errorf("all upstreams must have a version label for blue-green deployment")
		}
		versionMap[up.Version] = append(versionMap[up.Version], up)
	}

	if _, ok := versionMap[cfg.ActiveVersion]; !ok {
		return nil, fmt.Errorf("active version %s not found in upstreams", cfg.ActiveVersion)
	}

	return &BlueGreenStrategy{
		activeVersion: cfg.ActiveVersion,
		upstreams:     versionMap,
	}, nil
}

func (s *BlueGreenStrategy) SelectUpstream() (config.Upstream, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	activeUpstreams := s.upstreams[s.activeVersion]
	if len(activeUpstreams) == 0 {
		return config.Upstream{}, fmt.Errorf("no active upstreams for version %s", s.activeVersion)
	}

	// Round-robin among active version upstreams
	return activeUpstreams[rand.Intn(len(activeUpstreams))], nil
}

func (s *BlueGreenStrategy) UpdateUpstreams(upstreams []config.Upstream) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Rebuild version map
	versionMap := make(map[string][]config.Upstream)
	for _, up := range upstreams {
		if up.Version != "" {
			versionMap[up.Version] = append(versionMap[up.Version], up)
		}
	}
	s.upstreams = versionMap
}

// CanaryStrategy routes a percentage of traffic to the new version
type CanaryStrategy struct {
	canaryPercent int
	stableVersion string
	canaryVersion string
	stableUpstreams []config.Upstream
	canaryUpstreams []config.Upstream
	mu              sync.RWMutex
}

func NewCanaryStrategy(cfg config.DeploymentConfig, upstreams []config.Upstream) (*CanaryStrategy, error) {
	if cfg.CanaryPercent < 0 || cfg.CanaryPercent > 100 {
		return nil, fmt.Errorf("canary_percent must be between 0 and 100")
	}

	// Find versions - assume 2 versions for canary
	versions := make(map[string][]config.Upstream)
	for _, up := range upstreams {
		if up.Version == "" {
			return nil, fmt.Errorf("all upstreams must have a version label for canary deployment")
		}
		versions[up.Version] = append(versions[up.Version], up)
	}

	if len(versions) != 2 {
		return nil, fmt.Errorf("canary deployment requires exactly 2 versions, found %d", len(versions))
	}

	// Determine stable and canary versions (canary is the one with lower upstream count or newer)
	var stableVer, canaryVer string
	var stableUps, canaryUps []config.Upstream

	for ver, ups := range versions {
		if stableVer == "" {
			stableVer = ver
			stableUps = ups
		} else {
			canaryVer = ver
			canaryUps = ups
		}
	}

	// If canary has more upstreams, swap
	if len(canaryUps) > len(stableUps) {
		stableVer, canaryVer = canaryVer, stableVer
		stableUps, canaryUps = canaryUps, stableUps
	}

	return &CanaryStrategy{
		canaryPercent:   cfg.CanaryPercent,
		stableVersion:   stableVer,
		canaryVersion:   canaryVer,
		stableUpstreams: stableUps,
		canaryUpstreams: canaryUps,
	}, nil
}

func (s *CanaryStrategy) SelectUpstream() (config.Upstream, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Decide based on canary percentage
	if rand.Intn(100) < s.canaryPercent {
		// Route to canary
		if len(s.canaryUpstreams) == 0 {
			return config.Upstream{}, fmt.Errorf("no canary upstreams available")
		}
		return s.canaryUpstreams[rand.Intn(len(s.canaryUpstreams))], nil
	}

	// Route to stable
	if len(s.stableUpstreams) == 0 {
		return config.Upstream{}, fmt.Errorf("no stable upstreams available")
	}
	return s.stableUpstreams[rand.Intn(len(s.stableUpstreams))], nil
}

func (s *CanaryStrategy) UpdateUpstreams(upstreams []config.Upstream) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Rebuild version groups
	versions := make(map[string][]config.Upstream)
	for _, up := range upstreams {
		if up.Version != "" {
			versions[up.Version] = append(versions[up.Version], up)
		}
	}

	// Update stable and canary based on version names
	if stableUps, ok := versions[s.stableVersion]; ok {
		s.stableUpstreams = stableUps
	}
	if canaryUps, ok := versions[s.canaryVersion]; ok {
		s.canaryUpstreams = canaryUps
	}
}

// RollingStrategy uses weighted distribution based on upstream weights
type RollingStrategy struct {
	upstreams    []config.Upstream
	totalWeight  int
	mu           sync.RWMutex
}

func NewRollingStrategy(cfg config.DeploymentConfig, upstreams []config.Upstream) (*RollingStrategy, error) {
	totalWeight := 0
	for _, up := range upstreams {
		if up.Weight <= 0 {
			return nil, fmt.Errorf("all upstreams must have a positive weight for rolling deployment")
		}
		totalWeight += up.Weight
	}

	if totalWeight == 0 {
		return nil, fmt.Errorf("total weight must be greater than 0 for rolling deployment")
	}

	return &RollingStrategy{
		upstreams:   upstreams,
		totalWeight: totalWeight,
	}, nil
}

func (s *RollingStrategy) SelectUpstream() (config.Upstream, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.upstreams) == 0 {
		return config.Upstream{}, fmt.Errorf("no upstreams available")
	}

	// Weighted random selection
	r := rand.Intn(s.totalWeight)
	cumulative := 0

	for _, up := range s.upstreams {
		cumulative += up.Weight
		if r < cumulative {
			return up, nil
		}
	}

	// Fallback (should not reach here)
	return s.upstreams[0], nil
}

func (s *RollingStrategy) UpdateUpstreams(upstreams []config.Upstream) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.upstreams = upstreams
	totalWeight := 0
	for _, up := range upstreams {
		if up.Weight > 0 {
			totalWeight += up.Weight
		}
	}
	s.totalWeight = totalWeight
}

// RecreateStrategy routes to the new version only (old version is down)
type RecreateStrategy struct {
	activeVersion string
	upstreams     []config.Upstream
	mu            sync.RWMutex
}

func NewRecreateStrategy(cfg config.DeploymentConfig, upstreams []config.Upstream) (*RecreateStrategy, error) {
	if cfg.ActiveVersion == "" {
		return nil, fmt.Errorf("active_version is required for recreate deployment")
	}

	// Filter upstreams by active version
	var activeUpstreams []config.Upstream
	for _, up := range upstreams {
		if up.Version == cfg.ActiveVersion {
			activeUpstreams = append(activeUpstreams, up)
		}
	}

	if len(activeUpstreams) == 0 {
		return nil, fmt.Errorf("no upstreams found for active version %s", cfg.ActiveVersion)
	}

	return &RecreateStrategy{
		activeVersion: cfg.ActiveVersion,
		upstreams:     activeUpstreams,
	}, nil
}

func (s *RecreateStrategy) SelectUpstream() (config.Upstream, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.upstreams) == 0 {
		return config.Upstream{}, fmt.Errorf("no upstreams available for version %s", s.activeVersion)
	}

	// Random selection among active version upstreams
	return s.upstreams[rand.Intn(len(s.upstreams))], nil
}

func (s *RecreateStrategy) UpdateUpstreams(upstreams []config.Upstream) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Filter by active version
	var activeUpstreams []config.Upstream
	for _, up := range upstreams {
		if up.Version == s.activeVersion {
			activeUpstreams = append(activeUpstreams, up)
		}
	}
	s.upstreams = activeUpstreams
}
