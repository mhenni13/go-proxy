package ratelimit

import (

	"golang.org/x/time/rate"
)

func NewRateLimiter(rps int) *rate.Limiter {
	if rps <= 0 {
		rps = 1000
	}
	return rate.NewLimiter(rate.Limit(rps), rps)
}
