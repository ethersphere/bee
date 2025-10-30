package pushsync_test

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/pushsync"
)

func TestCalculateOverdraftBackoff_InitialBounds(t *testing.T) {
	const samples = 200

	base := time.Duration(float64(pushsync.OverDraftRefresh) * pushsync.OverDraftBackoffMultiplier)
	lower := time.Duration(float64(base) * (1 - pushsync.OverDraftJitterPercent))
	upper := time.Duration(float64(base) * (1 + pushsync.OverDraftJitterPercent))

	for range samples {
		got := pushsync.CalculateOverdraftBackoff(0)
		if got < lower {
			t.Fatalf("backoff below lower bound: got=%v lower=%v", got, lower)
		}
		if got > upper {
			t.Fatalf("backoff above upper bound: got=%v upper=%v", got, upper)
		}
		if got < pushsync.OverDraftRefresh {
			t.Fatalf("backoff below minimum refresh: got=%v min=%v", got, pushsync.OverDraftRefresh)
		}
	}
}

func TestCalculateOverdraftBackoff_MaxEdgeJittersAroundCap(t *testing.T) {
	const samples = 400

	current := pushsync.MaxOverDraftRefresh * 10
	base := pushsync.MaxOverDraftRefresh
	lower := time.Duration(float64(base) * (1 - pushsync.OverDraftJitterPercent))
	upper := time.Duration(float64(base) * (1 + pushsync.OverDraftJitterPercent))

	sawAboveMax := false
	sawBelowMax := false

	for range samples {
		got := pushsync.CalculateOverdraftBackoff(current)
		if got < lower {
			t.Fatalf("backoff below expected jitter band: got=%v lower=%v", got, lower)
		}
		if got > upper {
			t.Fatalf("backoff above expected jitter band: got=%v upper=%v", got, upper)
		}
		if got > base {
			sawAboveMax = true
		}
		if got < base {
			sawBelowMax = true
		}
	}

	if !sawAboveMax {
		t.Fatalf("expected at least one sample above maxOverDraftRefresh; base=%v", base)
	}
	if !sawBelowMax {
		t.Fatalf("expected at least one sample below maxOverDraftRefresh; base=%v", base)
	}
	t.Logf("seen above max: %v, seen below max: %v", sawAboveMax, sawBelowMax)
}

func TestCalculateOverdraftBackoff_NeverBelowMin(t *testing.T) {
	const samples = 200
	for i := range samples {
		got := pushsync.CalculateOverdraftBackoff(0)
		if got < pushsync.OverDraftRefresh {
			t.Fatalf("backoff below minimum refresh: got=%v min=%v", got, pushsync.OverDraftRefresh)
		}
		if i < 5 {
			t.Logf("min-check sample %d: delay=%v (min %v)", i, got, pushsync.OverDraftRefresh)
		}
	}
}
