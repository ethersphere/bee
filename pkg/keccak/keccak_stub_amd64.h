// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// CALL_ON_FRAME calls the XKCP C function cfn with SP moved to top, the top of
// the caller's reserved frame, so the C stack (which has no stack checks)
// grows down into that frame. R12 is callee-saved under the System V ABI and
// holds the original SP across the call. The arguments must already be in DI
// and SI. Shared by the stubs and the stack probe so that the probe measures
// exactly the instructions the stubs run.
#define CALL_ON_FRAME(cfn, top) \
	MOVQ SP, R12 \
	LEAQ top, AX \
	MOVQ AX, SP \
	CALL cfn(SB) \
	MOVQ R12, SP
