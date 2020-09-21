// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package encryption

import (
	"github.com/ethersphere/bee/pkg/encryption"
	"github.com/ethersphere/bee/pkg/file/pipeline"
)

type encryptionWriter struct {
	next pipeline.ChainWriter
	enc  encryption.ChunkEncrypter
}

func NewEncryptionWriter(encrypter encryption.ChunkEncrypter, next pipeline.ChainWriter) pipeline.ChainWriter {
	return &encryptionWriter{
		next: next,
		enc:  encrypter,
	}
}

// Write assumes that the span is prepended to the actual data before the write !
func (e *encryptionWriter) ChainWrite(p *pipeline.PipeWriteArgs) error {
	key, encryptedSpan, encryptedData, err := e.enc.EncryptChunk(p.Data)
	if err != nil {
		return err
	}
	c := make([]byte, len(encryptedSpan)+len(encryptedData))
	copy(c[:8], encryptedSpan)
	copy(c[8:], encryptedData)
	p.Data = c // replace the verbatim data with the encrypted data
	p.Key = key
	return e.next.ChainWrite(p)
}

func (e *encryptionWriter) Sum() ([]byte, error) {
	return e.next.Sum()
}
