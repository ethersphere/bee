package ratelimit

import (
	"errors"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

var ErrRateLimitExceeded = errors.New("rate limit exceeded")

type Limiter struct {
	mux     sync.Mutex
	limiter map[string]*rate.Limiter
	rate    rate.Limit
	burst   int
}

func New(r time.Duration, b int) *Limiter {
	return &Limiter{
		limiter: make(map[string]*rate.Limiter),
		rate:    rate.Every(r),
		burst:   b,
	}
}

func (l *Limiter) Allow(key string, count int) error {

	l.mux.Lock()
	defer l.mux.Unlock()

	limiter, ok := l.limiter[key]
	if !ok {
		limiter = rate.NewLimiter(l.rate, l.burst)
		l.limiter[key] = limiter
	}

	if limiter.AllowN(time.Now(), count) {
		return nil
	}

	return ErrRateLimitExceeded
}

func (l *Limiter) Clear(key string) {

	l.mux.Lock()
	defer l.mux.Unlock()

	delete(l.limiter, key)
}
