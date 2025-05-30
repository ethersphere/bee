package pusher

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/pushsync"
	"github.com/ethersphere/bee/v2/pkg/stabilization"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	olog "github.com/opentracing/opentracing-go/log"
)

type Service struct {
	networkID         uint64
	storer            Storer
	pushSyncer        pushsync.PushSyncer
	batchExist        postage.BatchExist
	logger            log.Logger
	quit              chan struct{}
	chunksWorkerQuitC chan struct{}
	inflight          *inflight
	attempts          *attempts
	smuggler          chan OpChan
}

func New(
	networkID uint64,
	storer Storer,
	pushSyncer pushsync.PushSyncer,
	batchExist postage.BatchExist,
	logger log.Logger,
	startupStabilizer stabilization.Subscriber,
	retryCount int,
) *Service {
	p := &Service{
		networkID:  networkID,
		storer:     storer,
		pushSyncer: pushSyncer,
		batchExist: batchExist,
		logger:     logger.WithName(loggerName).Register(),

		quit:              make(chan struct{}),
		chunksWorkerQuitC: make(chan struct{}),
		inflight:          newInflight(),
		attempts:          &attempts{retryCount: retryCount, attempts: make(map[string]int)},
		smuggler:          make(chan OpChan),
	}
	go p.chunksWorker(startupStabilizer)
	return p
}

// chunksWorker is a loop that keeps looking for chunks that are locally uploaded ( by monitoring pushIndex )
// and pushes them to the closest peer and get a receipt.
func (s *Service) chunksWorker(startupStabilizer stabilization.Subscriber) {
	defer close(s.chunksWorkerQuitC)

	sub, unsubscribe := startupStabilizer.Subscribe()
	defer unsubscribe()

	select {
	case <-sub:
		s.logger.Debug("node warmup check completed")
	case <-s.quit:
		return
	}

	var (
		ctx, cancel = context.WithCancel(context.Background())
		sem         = make(chan struct{}, ConcurrentPushes)
		cc          = make(chan *Op)
	)

	// inflight.set handles the backpressure for the maximum amount of inflight chunks
	// and duplicate handling.
	chunks, unsubscribe := s.storer.SubscribePush(ctx)
	defer func() {
		unsubscribe()
		cancel()
	}()

	var wg sync.WaitGroup

	push := func(op *Op) {
		var (
			err      error
			doRepeat bool
		)

		defer func() {
			// no peer was found which may mean that the node is suffering from connections issues
			// we must slow down the pusher to prevent constant retries
			if errors.Is(err, topology.ErrNotFound) {
				select {
				case <-time.After(time.Second * 5):
				case <-s.quit:
				}
			}

			wg.Done()
			<-sem
			if doRepeat {
				select {
				case cc <- op:
				case <-s.quit:
				}
			}
		}()

		spanCtx := ctx
		if op.Span != nil {
			spanCtx = tracing.WithContext(spanCtx, op.Span.Context())
		} else {
			op.Span = opentracing.NoopTracer{}.StartSpan("noOp")
		}

		if op.Direct {
			err = s.pushDirect(spanCtx, s.logger, op)
		} else {
			doRepeat, err = s.pushDeferred(spanCtx, s.logger, op)
		}

		if err != nil {
			ext.LogError(op.Span, err)
		} else {
			op.Span.LogFields(olog.Bool("success", true))
		}
	}

	go func() {
		for {
			select {
			case ch, ok := <-chunks:
				if !ok {
					chunks = nil
					continue
				}
				select {
				case cc <- &Op{Chunk: ch, Direct: false}:
				case <-s.quit:
					return
				}
			case apiC := <-s.smuggler:
				go func() {
					for {
						select {
						case op := <-apiC:
							select {
							case cc <- op:
							case <-s.quit:
								return
							}
						case <-s.quit:
							return
						}
					}
				}()
			case <-s.quit:
				return
			}
		}
	}()

	defer wg.Wait()

	for {
		select {
		case op := <-cc:
			idAddress, err := storage.IdentityAddress(op.Chunk)
			if err != nil {
				op.Err <- err
				continue
			}
			op.identityAddress = idAddress
			if s.inflight.set(idAddress, op.Chunk.Stamp().BatchID()) {
				if op.Direct {
					select {
					case op.Err <- nil:
					default:
						s.logger.Debug("chunk already in flight, skipping", "chunk", op.Chunk.Address())
					}
				}
				continue
			}
			select {
			case sem <- struct{}{}:
				wg.Add(1)
				go push(op)
			case <-s.quit:
				return
			}
		case <-s.quit:
			return
		}
	}
}
