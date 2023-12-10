package getter

import (
	"context"
	"fmt"
	"time"
)

var (
	StrategyTimeout = 500 * time.Millisecond // timeout for each strategy
)

type Strategy = int

const (
	NONE Strategy = iota // no prefetching and no decoding
	DATA                 // just retrieve data shards no decoding
	PROX                 // proximity driven selective fetching
	RACE                 // aggressive fetching racing all chunks
	strategyCnt
)

func (g *getter) prefetch(ctx context.Context, strategy int, strict bool) {
	if strict && strategy == NONE {
		return
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	run := func(s Strategy) error {
		if s == PROX { // NOT IMPLEMENTED
			return fmt.Errorf("strategy %d not implemented", s)
		}

		var stop <-chan time.Time
		if s < RACE {
			timer := time.NewTimer(StrategyTimeout)
			defer timer.Stop()
			stop = timer.C
		}

		prefetch(ctx, g, s)

		select {
		// successfully retrieved shardCnt number of chunks
		case <-g.ready:
		case <-stop:
			return fmt.Errorf("prefetching with strategy %d timed out", s)
		case <-ctx.Done():
			return nil
		}
		// call the erasure decoder
		// if decoding is successful terminate the prefetch loop
		return g.recover(ctx) // context to cancel when shardCnt chunks are retrieved
	}
	var err error
	for s := strategy; s == strategy || (err != nil && !strict && s < strategyCnt); s++ {
		err = run(s)
	}
}

// prefetch launches the retrieval of chunks based on the strategy
func prefetch(ctx context.Context, g *getter, s Strategy) {
	var m []int
	switch s {
	case NONE:
		return
	case DATA:
		// only retrieve data shards
		m = g.missing()
	case PROX:
		// proximity driven selective fetching
		// NOT IMPLEMENTED
	case RACE:
		// retrieve all chunks at once enabling race among chunks
		m = g.missing()
		for i := g.shardCnt; i < len(g.addrs); i++ {
			m = append(m, i)
		}
	}
	for _, i := range m {
		i := i
		g.wg.Add(1)
		go func() {
			g.fetch(ctx, i)
			g.wg.Done()
		}()
	}
}
