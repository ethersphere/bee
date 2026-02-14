//go:build !amd64 || purego

package keccak

import (
	"golang.org/x/crypto/sha3"
)

var hasAVX512 = false
var hasAVX2 = false

func keccak256x4(inputs *[4][]byte, outputs *[4]Hash256) {
	for i := 0; i < 4; i++ {
		if inputs[i] == nil {
			continue
		}
		h := sha3.NewLegacyKeccak256()
		h.Write(inputs[i])
		copy(outputs[i][:], h.Sum(nil))
	}
}

func keccak256x8(inputs *[8][]byte, outputs *[8]Hash256) {
	for i := 0; i < 8; i++ {
		if inputs[i] == nil {
			continue
		}
		h := sha3.NewLegacyKeccak256()
		h.Write(inputs[i])
		copy(outputs[i][:], h.Sum(nil))
	}
}
