//go:build !linux || !amd64 || purego

package keccak

func keccak256x4(_ *[4][]byte, _ *[4]Hash256) {
	panic("keccak: SIMD not available on this platform")
}

func keccak256x8(_ *[8][]byte, _ *[8]Hash256) {
	panic("keccak: SIMD not available on this platform")
}
