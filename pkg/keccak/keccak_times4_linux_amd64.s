//go:build amd64 && !purego

#include "textflag.h"
#include "funcdata.h"
#include "go_asm.h"
#include "keccak_stub_amd64.h"

// func keccak256x4(inputs *[4][]byte, outputs *[4]Hash256)
//
// The C code runs on the goroutine stack and does no stack checks, so it must
// not grow below this frame: the space under SP is only guaranteed to be a few
// hundred bytes, and overflowing it silently corrupts adjacent goroutine stacks
// or heap spans. SP is therefore moved to the top of the frame for the call so
// the C stack grows down into the reserved area (see CALL_ON_FRAME).
TEXT ·keccak256x4(SB), $4096-16
	NO_LOCAL_POINTERS
	MOVQ inputs+0(FP), DI
	MOVQ outputs+8(FP), SI
	CALL_ON_FRAME(go_keccak256x4, const_keccak256x4FrameSize(SP))
	RET
