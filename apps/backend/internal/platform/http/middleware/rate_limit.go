package middleware

import (
	"github.com/newrelic/go-agent/v3/newrelic"
)

type RateLimitMiddleware struct {
	nrApp *newrelic.Application
}

func NewRateLimitMiddleware(nrApp *newrelic.Application) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		nrApp: nrApp,
	}
}

func (r *RateLimitMiddleware) RecordRateLimitHit(endpoint string) {
	if r.nrApp != nil {
		r.nrApp.RecordCustomEvent("RateLimitHit", map[string]interface{}{
			"endpoint": endpoint,
		})
	}
}
