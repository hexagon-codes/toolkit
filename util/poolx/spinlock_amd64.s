//go:build amd64

#include "textflag.h"

// func procyieldAsm(cycles int)
// PAUSE instruction for x86-64, reduces power consumption and
// improves performance on spin-wait loops
TEXT ·procyieldAsm(SB),NOSPLIT,$0-8
    MOVQ    cycles+0(FP), AX
loop:
    PAUSE
    SUBQ    $1, AX
    JNZ     loop
    RET
