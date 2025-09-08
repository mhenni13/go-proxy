package ratelimit

import (
	"fmt"

	"golang.org/x/time/rate"
)

// NewRateLimiter creates a rate limiter from config
func NewRateLimiter(rps *int) (*rate.Limiter, error) {
	if rps == nil {
		// No rate limit → unlimited
		return rate.NewLimiter(rate.Inf, 0), nil
	}

	if *rps < 0 {
		return nil, fmt.Errorf("rate_limit cannot be negative: %d", *rps)
	}

	// Valid positive number → fixed limiter
	return rate.NewLimiter(rate.Limit(*rps), *rps), nil
}
