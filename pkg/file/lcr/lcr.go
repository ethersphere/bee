package lcr

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// LazyChunkReader implements LazySectionReader
type LazyChunkReader struct {
	ctx       context.Context
	addr      swarm.Address // root address
	chunkData []byte
	off       int64 // offset
	chunkSize int64 // inherit from chunker
	branches  int64 // inherit from chunker
	hashSize  int64 // inherit from chunker
	depth     int
	getter    storage.Getter
}

func Join(ctx context.Context, addr swarm.Address, getter storage.Getter, depth int) *LazyChunkReader {
	hashSize := int64(len(addr.Bytes()))
	return &LazyChunkReader{
		addr:      addr,
		chunkSize: swarm.ChunkSize,
		branches:  swarm.ChunkSize / hashSize,
		hashSize:  hashSize,
		depth:     depth,
		getter:    getter,
		ctx:       ctx,
	}
}

func (r *LazyChunkReader) Context() context.Context {
	return r.ctx
}

// Size is meant to be called on the LazySectionReader
func (r *LazyChunkReader) Size(ctx context.Context, quitC chan bool) (n int64, err error) {
	// metrics.GetOrRegisterCounter("lazychunkreader/size", nil).Inc(1)

	// var sp opentracing.Span
	// var cctx context.Context
	// cctx, sp = spancontext.StartSpan(
	// 	ctx,
	// 	"lcr.size")
	// defer sp.Finish()

	//log.Debug("lazychunkreader.size", "addr", r.addr)
	if r.chunkData == nil {
		//startTime := time.Now()
		chunk, err := r.getter.Get(ctx, storage.ModeGetRequest, r.addr)
		if err != nil {
			//metrics.GetOrRegisterResettingTimer("lcr/getter/get/err", nil).UpdateSince(startTime)
			return 0, err
		}
		//metrics.GetOrRegisterResettingTimer("lcr/getter/get", nil).UpdateSince(startTime)
		r.chunkData = chunk.Data()
	}

	s := chunkSize(r.chunkData)
	//log.Debug("lazychunkreader.size", "key", r.addr, "size", s)

	return int64(s), nil
}

// read at can be called numerous times
// concurrent reads are allowed
// Size() needs to be called synchronously on the LazyChunkReader first
func (r *LazyChunkReader) ReadAt(b []byte, off int64) (read int, err error) {
	//metrics.GetOrRegisterCounter("lazychunkreader/readat", nil).Inc(1)

	// var sp opentracing.Span
	// var cctx context.Context
	// cctx, sp = spancontext.StartSpan(
	// 	r.ctx,
	// 	"lcr.read")
	// defer sp.Finish()

	// defer func() {
	// 	sp.LogFields(
	// 		olog.Int("off", int(off)),
	// 		olog.Int("read", read))
	// }()

	// this is correct, a swarm doc cannot be zero length, so no EOF is expected
	if len(b) == 0 {
		return 0, nil
	}
	quitC := make(chan bool)
	size, err := r.Size(r.ctx, quitC)
	if err != nil {
		//log.Debug("lazychunkreader.readat.size", "size", size, "err", err)
		return 0, err
	}

	errC := make(chan error)

	// }
	var treeSize int64
	var depth int
	// calculate depth and max treeSize
	treeSize = r.chunkSize
	for ; treeSize < size; treeSize *= r.branches {
		depth++
	}
	wg := sync.WaitGroup{}
	length := int64(len(b))
	for d := 0; d < r.depth; d++ {
		off *= r.chunkSize
		length *= r.chunkSize
	}
	wg.Add(1)
	go r.join(r.ctx, b, off, off+length, depth, treeSize/r.branches, r.chunkData, &wg, errC, quitC)
	go func() {
		wg.Wait()
		close(errC)
	}()

	err = <-errC
	if err != nil {
		//log.Debug("lazychunkreader.readat.errc", "err", err)
		close(quitC)
		return 0, err
	}
	if off+int64(len(b)) >= size {
		//log.Debug("lazychunkreader.readat.return at end", "size", size, "off", off)
		return int(size - off), io.EOF
	}
	//log.Debug("lazychunkreader.readat.errc", "buff", len(b))
	return len(b), nil
}

func (r *LazyChunkReader) join(ctx context.Context, b []byte, off, eoff int64, depth int, treeSize int64, chunkData []byte, parentWg *sync.WaitGroup, errC chan error, quitC chan bool) {
	defer parentWg.Done()
	// find appropriate block level
	for chunkSize(chunkData) < uint64(treeSize) && depth > r.depth {
		treeSize /= r.branches
		depth--
	}

	// leaf chunk found
	if depth == r.depth {
		extra := 8 + eoff - int64(len(chunkData))
		if extra > 0 {
			eoff -= extra
		}
		copy(b, chunkData[8+off:8+eoff])
		return // simply give back the chunks reader for content chunks
	}

	// subtree
	start := off / treeSize
	end := (eoff + treeSize - 1) / treeSize

	// last non-leaf chunk can be shorter than default chunk size, let's not read it further then its end
	currentBranches := int64(len(chunkData)-8) / r.hashSize
	if end > currentBranches {
		end = currentBranches
	}

	wg := &sync.WaitGroup{}
	defer wg.Wait()
	for i := start; i < end; i++ {
		soff := i * treeSize
		roff := soff
		seoff := soff + treeSize

		if soff < off {
			soff = off
		}
		if seoff > eoff {
			seoff = eoff
		}
		if depth > 1 {
			wg.Wait()
		}
		wg.Add(1)
		go func(j int64) {
			childAddress := swarm.NewAddress(chunkData[8+j*r.hashSize : 8+(j+1)*r.hashSize])
			//startTime := time.Now()
			chunk, err := r.getter.Get(ctx, storage.ModeGetRequest, childAddress)
			if err != nil {
				//metrics.GetOrRegisterResettingTimer("lcr/getter/get/err", nil).UpdateSince(startTime)
				select {
				case errC <- fmt.Errorf("chunk %v-%v not found; key: %s", off, off+treeSize, fmt.Sprintf("%x", childAddress)):
				case <-quitC:
				}
				wg.Done()
				return
			}
			chunkData = chunk.Data()
			//metrics.GetOrRegisterResettingTimer("lcr/getter/get", nil).UpdateSince(startTime)
			if l := len(chunkData); l < 9 {
				select {
				case errC <- fmt.Errorf("chunk %v-%v incomplete; key: %s, data length %v", off, off+treeSize, fmt.Sprintf("%x", childAddress), l):
				case <-quitC:
				}
				wg.Done()
				return
			}
			if soff < off {
				soff = off
			}
			r.join(ctx, b[soff-off:seoff-off], soff-roff, seoff-roff, depth-1, treeSize/r.branches, chunkData, wg, errC, quitC)
		}(i)
	} //for
}

// Read keeps a cursor so cannot be called simulateously, see ReadAt
func (r *LazyChunkReader) Read(b []byte) (read int, err error) {
	//log.Trace("lazychunkreader.read", "key", r.addr)
	//metrics.GetOrRegisterCounter("lazychunkreader/read", nil).Inc(1)

	read, err = r.ReadAt(b, r.off)
	if err != nil && err != io.EOF {
		//log.Trace("lazychunkreader.readat", "read", read, "err", err)
		//metrics.GetOrRegisterCounter("lazychunkreader/read/err", nil).Inc(1)
		return read, err
	}

	//metrics.GetOrRegisterCounter("lazychunkreader/read/bytes", nil).Inc(int64(read))

	r.off += int64(read)
	return read, err
}

// completely analogous to standard SectionReader implementation
var errWhence = errors.New("seek: invalid whence")
var errOffset = errors.New("seek: invalid offset")

func (r *LazyChunkReader) Seek(offset int64, whence int) (int64, error) {
	// cctx, sp := spancontext.StartSpan(
	// 	r.ctx,
	// 	"lcr.seek")
	// defer sp.Finish()

	// log.Debug("lazychunkreader.seek", "key", r.addr, "offset", offset)
	switch whence {
	default:
		return 0, errWhence
	case 0:
		offset += 0
	case 1:
		offset += r.off
	case 2:

		if r.chunkData == nil { //seek from the end requires rootchunk for size. call Size first
			_, err := r.Size(r.ctx, nil)
			if err != nil {
				return 0, fmt.Errorf("chunk size: %w", err)
			}
		}
		offset += int64(chunkSize(r.chunkData))
	}

	if offset < 0 {
		return 0, errOffset
	}
	r.off = offset
	return offset, nil
}

func chunkSize(data []byte) uint64 {
	return binary.LittleEndian.Uint64(data[:8])
}
