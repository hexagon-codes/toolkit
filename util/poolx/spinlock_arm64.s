//go:build arm64

#include "textflag.h"

// func procyieldAsm(cycles int)
// YIELD instruction for ARM64, hints the processor that the current
// thread is in a spin-wait loop
TEXT ·procyieldAsm(SB),NOSPLIT,$0-8
    MOVD    cycles+0(FP), R0
loop:
    YIELD
    SUB     $1, R0, R0
    CBNZ    R0, loop
    RET
