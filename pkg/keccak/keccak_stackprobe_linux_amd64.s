//go:build amd64 && !purego

#include "textflag.h"
#include "funcdata.h"
#include "go_asm.h"
#include "keccak_stub_amd64.h"

// The probe frame is filled with the caller-supplied pattern rather than a
// fixed one: a stray write that ORs or ANDs bits into a byte which already
// holds them leaves the byte unchanged, so callers run the probe with
// complementary patterns to make every write visible.
//
// The probe frame is 65536 bytes (8192 quadwords). The call-site SP is placed
// 32+off bytes below its top. Rather than CALLing the stub, the probe lays out
// the stub's frame itself (return address and saved frame pointer, then
// frameSize bytes) and runs the stub body through the shared CALL_ON_FRAME
// macro. A real CALL would run the stub's stack-check prologue while SP points
// into the probe frame: if the goroutine is preempted there, the GC cannot
// unwind through the probe's SP write and the runtime throws. The C code has
// no prologue and assembly is never async-preempted, so inlining the body
// keeps the goroutine unstoppable for the duration of the probe. R13 and R14
// are callee-saved under the System V ABI and survive the C call.
#define STACK_PROBE(cfn, frameSize) \
	NO_LOCAL_POINTERS \
	MOVQ SP, DI \
	MOVQ $8192, CX \
	MOVQ pattern+24(FP), AX \
	REP; STOSQ \
	MOVQ inputs+0(FP), DI \
	MOVQ outputs+8(FP), SI \
	MOVQ off+16(FP), BX \
	MOVQ SP, R13 \
	LEAQ 65504(SP), R14 \
	SUBQ BX, R14 \
	MOVQ R14, AX \
	SUBQ $16, AX \
	SUBQ $frameSize, AX \
	MOVQ AX, SP \
	CALL_ON_FRAME(cfn, frameSize(SP)) \
	MOVQ R13, SP \
	MOVQ R14, R9 \
	MOVQ SP, DI \
	MOVQ $8192, CX \
	MOVQ pattern+24(FP), AX \
	REP; SCASQ \
	JNE 3(PC) \
	MOVQ $0, ret+32(FP) \
	RET \
	SUBQ $8, DI \
	SUBQ DI, R9 \
	MOVQ R9, ret+32(FP) \
	RET

// func stackProbe4(inputs *[4][]byte, outputs *[4]Hash256, off, pattern uintptr) uintptr
TEXT ·stackProbe4(SB), $65536-40
	STACK_PROBE(go_keccak256x4, const_keccak256x4FrameSize)

// func stackProbe8(inputs *[8][]byte, outputs *[8]Hash256, off, pattern uintptr) uintptr
TEXT ·stackProbe8(SB), $65536-40
	STACK_PROBE(go_keccak256x8, const_keccak256x8FrameSize)
