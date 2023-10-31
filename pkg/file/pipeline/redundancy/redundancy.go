// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redundancy

import (
	"github.com/ethersphere/bee/pkg/file/pipeline"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/klauspost/reedsolomon"
)

type Level uint8

const (
	NONE Level = iota
	MEDIUM
	STRONG
	INSANE
	PARANOID
)

const maxLevel = 8

// GetParities returns number of parities based on appendix F table 5
func (l Level) GetParities(shards int) int {
	switch l {
	case PARANOID:
		return 91
	case INSANE:
		return 31
	case STRONG:
		switch s := shards; {
		case s >= 104:
			return 21
		case s >= 95:
			return 20
		case s >= 86:
			return 19
		case s >= 77:
			return 18
		case s >= 69:
			return 17
		case s >= 61:
			return 16
		case s >= 53:
			return 15
		case s >= 46:
			return 14
		case s >= 39:
			return 13
		case s >= 32:
			return 12
		case s >= 26:
			return 11
		case s >= 20:
			return 10
		case s >= 15:
			return 9
		case s >= 10:
			return 8
		case s >= 6:
			return 7
		case s >= 3:
			return 6
		default:
			return 5
		}
	case MEDIUM:
		switch s := shards; {
		case s >= 94:
			return 9
		case s >= 68:
			return 8
		case s >= 46:
			return 7
		case s >= 28:
			return 6
		case s >= 14:
			return 5
		case s >= 5:
			return 4
		default:
			return 3
		}
	default:
		return 0
	}
}

// GetMaxShards returns back the maximum number of effective data chunks
func (l Level) GetMaxShards() int {
	p := l.GetParities(swarm.Branches)
	return swarm.Branches - p
}

// GetEncParities returns number of parities for encrypted chunks based on appendix F table 6
func (l Level) GetEncParities(shards int) int {
	switch l {
	case PARANOID:
		return 56
	case INSANE:
		return 27
	case STRONG:
		return 19
	case MEDIUM:
		return 9
	default:
		return 0
	}
}

// GetMaxEncShards returns back the maximum number of effective encrypted data chunks
func (l Level) GetMaxEncShards() int {
	p := l.GetParities(swarm.EncryptedBranches)
	return (swarm.Branches - p) / 2
}

type parityChunkCallback func(level int, span, address, key []byte) error

type Params struct {
	level      Level
	pipeLine   pipeline.PipelineFunc
	buffer     [][][]byte // keeps bytes of chunks on each level for producing erasure coded data; [levelIndex][branchIndex][byteIndex]
	cursor     []int      // index of the current buffered chunk in Buffer. this is basically the latest used branchIndex.
	maxShards  int        // number of chunks after which the parity encode function should be called
	maxParity  int        // number of parity chunks if maxShards has been reached for erasure coding
	encryption bool
}

func New(level Level, encryption bool, pipeLine pipeline.PipelineFunc) *Params {
	// init dataBuffer for erasure coding
	rsChunkLevels := 0
	if level != NONE {
		rsChunkLevels = maxLevel
	}
	Buffer := make([][][]byte, rsChunkLevels)
	for i := 0; i < rsChunkLevels; i++ {

		Buffer[i] = make([][]byte, swarm.Branches) // not exact
	}

	maxShards := 0
	maxParity := 0
	if encryption {
		maxShards = level.GetMaxEncShards()
		maxParity = level.GetParities(swarm.EncryptedBranches)
	} else {
		maxShards = level.GetMaxShards()
		maxParity = level.GetParities(swarm.BmtBranches)
	}

	return &Params{
		level:      level,
		pipeLine:   pipeLine,
		buffer:     Buffer,
		cursor:     make([]int, 9),
		maxShards:  maxShards,
		maxParity:  maxParity,
		encryption: encryption,
	}
}

// ACCESSORS

func (p *Params) MaxShards() int {
	return p.maxShards
}

func (p *Params) Level() Level {
	return p.level
}

// METHODS

func (p *Params) Parity(shards int) int {
	if p.encryption {
		return p.level.GetEncParities(shards)
	}
	return p.level.GetParities(shards)
}

// ChainWrite caches the chunk data on the given chunk level and if it is full then it calls Encode
func (p *Params) ChainWrite(chunkLevel int, span, ref, data []byte, callback parityChunkCallback) error {
	// append chunk to the buffer
	p.buffer[chunkLevel][p.cursor[chunkLevel]] = data
	p.cursor[chunkLevel]++

	// add parity chunk if it is necessary
	if p.cursor[chunkLevel] == p.maxShards {
		// append erasure coded data
		return p.Encode(chunkLevel, callback)
	}
	return nil
}

// Encode produces and stores parity chunks that will be also passed back to the caller
func (p *Params) Encode(chunkLevel int, callback parityChunkCallback) error {
	shards := p.cursor[chunkLevel]
	parities := p.Parity(shards)
	enc, err := reedsolomon.New(shards, parities)
	if err != nil {
		return err
	}
	n := shards + parities

	// alloc for parity chunks
	for i := shards; i < n; i++ {
		p.buffer[chunkLevel][i] = make([]byte, swarm.ChunkWithSpanSize)
	}
	// caculate parity chunks
	err = enc.Encode(p.buffer[chunkLevel][:n])
	if err != nil {
		return err
	}
	// store and pass newly created parity chunks
	for i := shards; i < n; i++ {
		chunkData := p.buffer[chunkLevel][i]
		span := chunkData[:swarm.SpanSize]

		// store data chunk
		writer := p.pipeLine()
		args := pipeline.PipeWriteArgs{
			Data: chunkData,
			Span: span,
		}
		err = writer.ChainWrite(&args)
		if err != nil {
			return err
		}

		// write parity chunk to the level above
		callback(chunkLevel+1, span, args.Ref, args.Key)
	}
	// reset cursor of dataBuffer in case it was a full chunk
	p.cursor[chunkLevel] = 0

	return nil
}
