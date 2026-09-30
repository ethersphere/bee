//go:build amd64 && !purego

#include "textflag.h"
#include "funcdata.h"
#include "go_asm.h"

// func keccak256x8(inputs *[8][]byte, outputs *[8]Hash256)
//
// See keccak_times4_linux_amd64.s: SP is moved to the top of the frame so the
// C code's stack grows down into the reserved area instead of below it.
TEXT ·keccak256x8(SB), $4096-16
	NO_LOCAL_POINTERS
	MOVQ inputs+0(FP), DI
	MOVQ outputs+8(FP), SI
	MOVQ SP, R12
	LEAQ const_keccak256x8FrameSize(SP), AX
	MOVQ AX, SP
	CALL go_keccak256x8(SB)
	MOVQ R12, SP
	RET
