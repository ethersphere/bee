// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	crand "crypto/rand"
	"io"

	"github.com/ethersphere/bee/pkg/postage"
)

const signatureSize = 65

// MustNewSignature will create a new random signature (65 byte slice). Panics
// on errors.
func MustNewSignature() []byte {
	sig := make([]byte, signatureSize)
	_, err := io.ReadFull(crand.Reader, sig)
	if err != nil {
		panic(err)
	}
	return sig
}

// MustNewStamp will generate a postage stamp with random data. Panics on
// errors.
func MustNewStamp() *postage.Stamp {
	return postage.NewStamp(MustNewID(), MustNewID()[:8], MustNewID()[:8], MustNewSignature())
}
