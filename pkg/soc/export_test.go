// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package soc

var (
	ErrInvalidAddress = errInvalidAddress
	Hash              = hash
	RecoverAddress    = recoverAddress
)

// Signature returns the SOC signature.
func (s *SOC) Signature() []byte {
	return s.signature
}

// OwnerAddress returns the ethereum address of the SOC owner.
func (s *SOC) OwnerAddress() []byte {
	return s.owner
}

// ID returns the SOC id.
func (s *SOC) ID() []byte {
	return s.id
}
