// Shim for Go >= 1.21 — provides the same assembly as runtime_amd64.s
// without the !go1.21 build constraint.

//go:build amd64 && go1.21 && !goexperiment.staticlockranking

#include "textflag.h"

TEXT ·addrOfSpinning(SB),NOSPLIT,$0-8
	LEAQ runtime·sched(SB), AX
	ADDQ $92, AX
	MOVQ AX, ret+0(FP)
	RET
