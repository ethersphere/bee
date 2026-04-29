//go:build amd64 && !purego

#include "textflag.h"

// func keccak256x8(inputs *[8][]byte, outputs *[8]Hash256)
// Frame size 16384: AVX-512 state is larger (25 x 64 bytes = 1600 bytes) and
// the permutation uses more stack. Generous headroom provided.
TEXT ·keccak256x8(SB), $16384-16
    MOVQ inputs+0(FP), DI
    MOVQ outputs+8(FP), SI
    CALL go_keccak256x8(SB)
    RET
