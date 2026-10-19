// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"errors"
	"time"

	"github.com/ethersphere/bee/v2/pkg/pusher"
	"github.com/ethersphere/bee/v2/pkg/pushsync"
	"github.com/ethersphere/bee/v2/pkg/safe"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/sync/errgroup"
)

// directUploadRetryDelay is the time a direct upload waits for peers before
// it submits the chunk to the pusher again.
const directUploadRetryDelay = 5 * time.Second

// DirectUpload is the implementation of the NetStore.DirectUpload method.
func (db *DB) DirectUpload() PutterSession {
	// egCtx will allow early exit of Put operations if we have
	// already encountered error.
	eg, egCtx := errgroup.WithContext(context.Background())

	// chunks that failed once wait here for one more push at the end of the
	// upload
	q := newRequeue(directUploadRequeueLimit)

	return &putterSession{
		Putter: putterWithMetrics{
			storage.PutterFunc(func(ctx context.Context, ch swarm.Chunk) error {
				// the upload has already failed: stop it here instead of
				// pushing the rest of its chunks
				if egCtx.Err() != nil {
					return context.Cause(egCtx)
				}
				select {
				case db.directUploadLimiter <- struct{}{}:
				case <-egCtx.Done():
					return context.Cause(egCtx)
				}
				eg.Go(safe.RunFunc(db.logger, "storer-netstore-direct-upload", func() error {
					defer func() { <-db.directUploadLimiter }()

					err := db.pushChunk(ctx, egCtx, ch)
					if !errors.Is(err, pusher.ErrCouldNotSync) {
						return err
					}
					db.metrics.DirectUploadRequeued.Inc()
					db.logger.Debug("direct upload: chunk failed, will push it again at the end of the upload", "chunk_address", ch.Address())

					oldest, full := q.add(requeued{ctx: ctx, ch: ch})
					if !full {
						return nil
					}
					return db.pushRequeued(egCtx, oldest)
				}))
				return nil
			}),
			db.metrics,
			"netstore",
		},
		done: func(swarm.Address) error {
			if err := eg.Wait(); err != nil {
				return err
			}
			items := q.drain()
			if len(items) == 0 {
				return nil
			}
			rg, rgCtx := errgroup.WithContext(context.Background())
			for _, r := range items {
				select {
				case db.directUploadLimiter <- struct{}{}:
				case <-rgCtx.Done():
					return rg.Wait()
				}
				rg.Go(safe.RunFunc(db.logger, "storer-netstore-direct-upload-requeued", func() error {
					defer func() { <-db.directUploadLimiter }()
					return db.pushRequeued(rgCtx, r)
				}))
			}
			return rg.Wait()
		},
		cleanup: func() error { _ = eg.Wait(); return nil },
	}
}

// pushRequeued pushes a chunk that failed once for the second and last time.
func (db *DB) pushRequeued(egCtx context.Context, r requeued) error {
	err := db.pushChunk(r.ctx, egCtx, r.ch)
	if err != nil {
		db.metrics.DirectUploadRequeueFailed.Inc()
		db.logger.Debug("direct upload: chunk failed again", "chunk_address", r.ch.Address(), "error", err)
	}
	return err
}

// pushChunk pushes one chunk of a direct upload until it is synced or fails
// with an error a resubmit cannot fix.
func (db *DB) pushChunk(ctx, egCtx context.Context, ch swarm.Chunk) (err error) {
	span, logger, ctx := db.tracer.FollowSpanFromContext(ctx, "put-direct-upload", db.logger)
	span.SetAttributes(attribute.String("swarm.chunk.address", ch.Address().String()))
	defer func() {
		if err != nil {
			tracing.RecordError(span, err)
		}
		span.End()
	}()

	for {
		// the select below picks at random among ready cases, so check the
		// contexts first: a gone client must not get another push
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := egCtx.Err(); err != nil {
			return err
		}
		op := &pusher.Op{Chunk: ch, Err: make(chan error, 1), Direct: true, Span: span}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-egCtx.Done():
			return egCtx.Err()
		case <-db.quit:
			return ErrDBQuit
		case db.pusherFeed <- op:
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-egCtx.Done():
				return egCtx.Err()
			case <-db.quit:
				return ErrDBQuit
			case err := <-op.Err:
				if errors.Is(err, pushsync.ErrShallowReceipt) {
					logger.Debug("direct upload: shallow receipt received, retrying", "chunk", ch.Address())
				} else if errors.Is(err, topology.ErrNotFound) {
					logger.Debug("direct upload: no peers available, retrying", "chunk", ch.Address())
					// wait for peers instead of resubmitting in a tight loop
					select {
					case <-time.After(directUploadRetryDelay):
					case <-ctx.Done():
						return ctx.Err()
					case <-egCtx.Done():
						return egCtx.Err()
					case <-db.quit:
						return ErrDBQuit
					}
				} else {
					return err
				}
			}
		}
	}
}

// Download is the implementation of the NetStore.Download method.
func (db *DB) Download(cache bool) storage.Getter {
	return getterWithMetrics{
		storage.GetterFunc(func(ctx context.Context, address swarm.Address) (ch swarm.Chunk, err error) {
			span, logger, ctx := db.tracer.StartSpanFromContext(ctx, "get-chunk", db.logger)
			span.SetAttributes(attribute.String("swarm.chunk.address", address.String()))
			defer func() {
				if err != nil {
					tracing.RecordError(span, err)
				} else {
					span.SetAttributes(attribute.Bool("swarm.operation.success", true))
				}
				span.End()
			}()

			ch, err = db.Lookup().Get(ctx, address)
			switch {
			case err == nil:
				span.AddEvent("chunk found locally")
				return ch, nil
			case errors.Is(err, storage.ErrNotFound):
				span.AddEvent("retrieve chunk from network")
				if db.retrieval != nil {
					// if chunk is not found locally, retrieve it from the network
					ch, err = db.retrieval.RetrieveChunk(ctx, address, swarm.ZeroAddress)
					if err == nil && cache {
						select {
						case <-ctx.Done():
						case <-db.quit:
						case db.cacheLimiter.sem <- struct{}{}:
							db.cacheLimiter.wg.Add(1)
							go func() {
								defer func() {
									<-db.cacheLimiter.sem
									db.cacheLimiter.wg.Done()
								}()

								err := db.Cache().Put(db.cacheLimiter.ctx, ch)
								if err != nil {
									logger.Debug("putting chunk to cache failed", "error", err, "chunk_address", ch.Address())
								}
							}()
						}
					}
				}
			}
			if err != nil {
				return nil, err
			}
			return ch, nil
		}),
		db.metrics,
		"netstore",
	}
}

// PusherFeed is the implementation of the NetStore.PusherFeed method.
func (db *DB) PusherFeed() <-chan *pusher.Op {
	return db.pusherFeed
}
