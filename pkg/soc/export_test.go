// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package soc

var (
	ErrInvalidAddress = errInvalidAddress
	Hash              = hash
	RecoverAddress    = recoverAddress
)

// Signature returns the soc signature.
func (s *Soc) Signature() []byte {
	return s.signature
}

// OwnerAddress returns the ethereum address of the signer of the Chunk.
func (s *Soc) OwnerAddress() []byte {
	return s.owner
}

// Id returns the soc id.
func (s *Soc) ID() []byte {
	return s.id
}
