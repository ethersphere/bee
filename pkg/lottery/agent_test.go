package lottery_test

import (
	"errors"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/lottery"
	"github.com/ethersphere/bee/pkg/swarm"
)

type TestSampler struct {
	calculate func(salt []byte, sd uint8) (rc []byte, err error)
}

func (s *TestSampler) WithCalculate(f func(salt []byte, sd uint8) (rc []byte, err error)) *TestSampler {
	s.calculate = f
	return s
}
func (s *TestSampler) Calculate(salt []byte, sd uint8) (rc []byte, err error) {
	return s.calculate(salt, sd)
}

type TestDepthMonitor struct{}

func (*TestDepthMonitor) StorageDepth() uint8 {
	return 13
}

func TestClaim(t *testing.T) {

	for _, tc := range []struct {
		desc           string
		slashed        bool
		winning        bool
		isWinningError error
		claimed        bool
	}{
		{
			desc:           "winner claims after claim phase starts",
			slashed:        false,
			winning:        true,
			isWinningError: nil,
			claimed:        true,
		},
		{
			desc:           "slashed node makes no claim",
			slashed:        true,
			winning:        true, // (!irrelevant but even if true)
			isWinningError: nil,
			claimed:        false,
		},
		{
			desc:           "non-slahed node that does not win makes no claim",
			slashed:        false,
			winning:        false,
			isWinningError: nil,
			claimed:        false,
		},
		{
			desc:           "non-slahed node that does not win makes no claim",
			slashed:        false,
			winning:        false,
			isWinningError: errors.New("isWinningError"),
			claimed:        false,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			claimCalled := make(chan struct{}, 2)
			c := &TestContract{}
			c = c.WithConfig(func() (startBlock, blocksPerRound, BlocksPerPhase uint64) {
				return 100, 12, 3
			}).WithClaim(func() error {
				claimCalled <- struct{}{}
				return nil
			}).WithIsWinner(func(overlay, truthSel, winnerSel []byte) (bool, bool, error) {
				return tc.slashed, tc.winning, tc.isWinningError
			}).WithRandomSeedFor(func(i int) ([]byte, error) {
				return hash([]byte{uint8(i)}), nil
			}).WithIsPlaying(func(overlay []byte, sd uint8, source []byte) (bool, error) {
				return false, nil
			})

			var rc []byte
			s := &TestSampler{}
			s = s.WithCalculate(func(salt []byte, sd uint8) (rc []byte, err error) {
				rc = hash([]byte{133})
				return rc, nil
			})

			d := &TestDepthMonitor{}

			quit := make(chan struct{})
			overlay := make([]byte, 32)
			blockheightC := make(chan uint64, 1)
			_ = rc

			a := lottery.New(c, d, s, overlay, blockheightC, quit)
			defer close(quit)
			startBlock, _, blocksPerPhase := a.Config()

			i := startBlock
			for ; i < startBlock+2*blocksPerPhase; i++ {
				blockheightC <- i
				select {
				case <-time.After(100 * time.Millisecond):
				case <-claimCalled:
					t.Fatalf("claim expected to be called after %v. but got it after %v.", 2*blocksPerPhase, i)
				}
			}
			if tc.claimed {
				blockheightC <- i
				select {
				case <-time.After(100 * time.Millisecond):
					t.Fatalf("claim expected to be called after %v. but it was not.", 2*blocksPerPhase)
				case <-claimCalled:
				}
				i++
			}

			blockheightC <- i
			select {
			case <-time.After(100 * time.Millisecond):
			case <-claimCalled:
				t.Fatalf("claim not xpected to be called after %v. but it was at %v.", 2*blocksPerPhase	+1, i)
			}

		})

	}
}

func hash(s []byte) []byte {
	h := swarm.NewHasher()
	h.Write(s)
	return h.Sum(nil)
}
