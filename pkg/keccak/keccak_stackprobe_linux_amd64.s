//go:build amd64 && !purego

#include "textflag.h"
#include "funcdata.h"

#define PROBE_PATTERN $0xA5A5A5A5A5A5A5A5

// The probe frame is 65536 bytes (8192 quadwords). The call-site SP is placed
// 32+off bytes below its top, leaving room for the two stub arguments and the
// saved probe SP, which live above the call-site SP and do not count as usage.
#define STACK_PROBE(stub) \
	NO_LOCAL_POINTERS \
	MOVQ SP, DI \
	MOVQ $8192, CX \
	MOVQ PROBE_PATTERN, AX \
	REP; STOSQ \
	MOVQ inputs+0(FP), DX \
	MOVQ outputs+8(FP), SI \
	MOVQ off+16(FP), BX \
	MOVQ SP, R8 \
	LEAQ 65504(SP), AX \
	SUBQ BX, AX \
	MOVQ AX, SP \
	MOVQ DX, 0(SP) \
	MOVQ SI, 8(SP) \
	MOVQ R8, 16(SP) \
	CALL stub(SB) \
	MOVQ SP, R9 \
	MOVQ 16(SP), SP \
	MOVQ SP, DI \
	MOVQ $8192, CX \
	MOVQ PROBE_PATTERN, AX \
	REP; SCASQ \
	JNE 3(PC) \
	MOVQ $0, ret+24(FP) \
	RET \
	SUBQ $8, DI \
	SUBQ DI, R9 \
	MOVQ R9, ret+24(FP) \
	RET

// func stackProbe4(inputs *[4][]byte, outputs *[4]Hash256, off uintptr) uintptr
TEXT ·stackProbe4(SB), $65536-32
	STACK_PROBE(·keccak256x4)

// func stackProbe8(inputs *[8][]byte, outputs *[8]Hash256, off uintptr) uintptr
TEXT ·stackProbe8(SB), $65536-32
	STACK_PROBE(·keccak256x8)
