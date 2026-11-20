package auth

import (
	"sync"
	"time"
)

// RateLimiter is a fixed-window per-key limiter (used for login attempts).
type RateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string]*window
	now    func() time.Time
}

type window struct {
	start time.Time
	count int
}

func NewRateLimiter(limit int, per time.Duration) *RateLimiter {
	return &RateLimiter{limit: limit, window: per, hits: map[string]*window{}, now: time.Now}
}

// Allow records a hit for key and reports whether it is within the limit.
func (r *RateLimiter) Allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	w, ok := r.hits[key]
	if !ok || now.Sub(w.start) >= r.window {
		r.hits[key] = &window{start: now, count: 1}
		if len(r.hits) > 10000 { // bound memory
			for k, v := range r.hits {
				if now.Sub(v.start) >= r.window {
					delete(r.hits, k)
				}
			}
		}
		return true
	}
	w.count++
	return w.count <= r.limit
}
