package ratelimit_test

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/ratelimit"
)

func TestRateLimit(t *testing.T) {

	var (
		key1  = "test1"
		key2  = "test2"
		rate  = time.Second
		burst = 10
	)

	limiter := ratelimit.New(rate, burst)

	err := limiter.Allow(key1, burst)
	if err != nil {
		t.Fatal(err)
	}

	err = limiter.Allow(key1, burst)
	if err == nil {
		t.Fatalf("want rate limit exceeded error")
	}

	limiter.Clear(key1)

	err = limiter.Allow(key1, burst)
	if err != nil {
		t.Fatal(err)
	}

	err = limiter.Allow(key2, burst)
	if err != nil {
		t.Fatal(err)
	}
}
