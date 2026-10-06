//go:build amd64 && !purego

#include "textflag.h"
#include "funcdata.h"
#include "go_asm.h"
#include "keccak_stub_amd64.h"

// func keccak256x8(inputs *[8][]byte, outputs *[8]Hash256)
//
// See keccak_times4_linux_amd64.s: SP is moved to the top of the frame so the
// C code's stack grows down into the reserved area instead of below it.
TEXT ·keccak256x8(SB), $4096-16
	NO_LOCAL_POINTERS
	MOVQ inputs+0(FP), DI
	MOVQ outputs+8(FP), SI
	CALL_ON_FRAME(go_keccak256x8, const_keccak256x8FrameSize(SP))
	RET
