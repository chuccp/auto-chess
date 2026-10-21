//go:build amd64 && !purego

#include "textflag.h"

DATA ·avxOnes<>+0(SB)/8, $0x0001000100010001
DATA ·avxOnes<>+8(SB)/8, $0x0001000100010001
DATA ·avxOnes<>+16(SB)/8, $0x0001000100010001
DATA ·avxOnes<>+24(SB)/8, $0x0001000100010001
GLOBL ·avxOnes<>(SB), RODATA, $32

// func fc0AVX2(input *uint8, weights *int8, bias *int32, out *int32)
TEXT ·fc0AVX2(SB), NOSPLIT, $0-32
	MOVQ input+0(FP), SI
	MOVQ weights+8(FP), DI
	MOVQ bias+16(FP), BX
	MOVQ out+24(FP), DX
	XORQ R9, R9
outer:
	VPXOR Y0, Y0, Y0
	XORQ R10, R10
inner:
	VMOVDQU (SI)(R10*1), Y1
	MOVQ R9, AX
	SHLQ $10, AX
	ADDQ R10, AX
	VPMADDUBSW (DI)(AX*1), Y1, Y2
	VPMADDWD ·avxOnes<>(SB), Y2, Y2
	VPADDD Y2, Y0, Y0
	ADDQ $32, R10
	CMPQ R10, $1024
	JLT inner

	VEXTRACTI128 $1, Y0, X1
	VPADDD X1, X0, X0
	VPSHUFD $0x4E, X0, X1
	VPADDD X1, X0, X0
	VPSHUFD $0xB1, X0, X1
	VPADDD X1, X0, X0
	VMOVD X0, R11
	MOVL (BX)(R9*4), R12
	ADDL R12, R11
	MOVL R11, (DX)(R9*4)
	INCQ R9
	CMPQ R9, $16
	JLT outer
	VZEROUPPER
	RET

// func applyDeltaAVX2(values *int32, add, remove *int16, n int)
TEXT ·applyDeltaAVX2(SB), NOSPLIT, $0-32
	MOVQ values+0(FP), DI
	MOVQ add+8(FP), SI
	MOVQ remove+16(FP), BX
	MOVQ n+24(FP), CX
	XORQ AX, AX
loop:
	VPMOVSXWD (SI)(AX*2), Y0
	VPMOVSXWD (BX)(AX*2), Y1
	VPSUBD Y1, Y0, Y0
	VPADDD (DI)(AX*4), Y0, Y0
	VMOVDQU Y0, (DI)(AX*4)
	ADDQ $8, AX
	CMPQ AX, CX
	JLT loop
	VZEROUPPER
	RET
