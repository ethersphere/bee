//go:build amd64 && !purego

#include "textflag.h"

// func keccak256x4(inputs *[4][]byte, outputs *[4]Hash256)
TEXT ·keccak256x4(SB), $8192-16
    MOVQ inputs+0(FP), DI
    MOVQ outputs+8(FP), SI
    CALL go_keccak256x4(SB)
    RET
