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

DATA ·avxZero<>+0(SB)/8, $0
DATA ·avxZero<>+8(SB)/8, $0
DATA ·avxZero<>+16(SB)/8, $0
DATA ·avxZero<>+24(SB)/8, $0
GLOBL ·avxZero<>(SB), RODATA, $32

DATA ·avx127<>+0(SB)/8, $0x0000007f0000007f
DATA ·avx127<>+8(SB)/8, $0x0000007f0000007f
DATA ·avx127<>+16(SB)/8, $0x0000007f0000007f
DATA ·avx127<>+24(SB)/8, $0x0000007f0000007f
GLOBL ·avx127<>(SB), RODATA, $32

DATA ·avxByteMask<>+0(SB)/8, $0x808080800c080400
DATA ·avxByteMask<>+8(SB)/8, $0x8080808080808080
GLOBL ·avxByteMask<>(SB), RODATA, $16

// func transformAVX2(values *int32, out *uint8)
TEXT ·transformAVX2(SB), NOSPLIT, $0-16
	MOVQ values+0(FP), SI
	MOVQ out+8(FP), DI
	VMOVDQU ·avxZero<>(SB), Y2
	VMOVDQU ·avx127<>(SB), Y3
	VMOVDQU ·avxByteMask<>(SB), X4
	XORQ R8, R8
perspLoop:
	MOVQ R8, AX
	SHLQ $12, AX
	ADDQ SI, AX
	MOVQ R8, BX
	SHLQ $9, BX
	ADDQ DI, BX
	XORQ R9, R9
transformLoop:
	VMOVDQU (AX)(R9*4), Y0
	VPMAXSD Y2, Y0, Y0
	VPMINSD Y3, Y0, Y0
	VMOVDQU 2048(AX)(R9*4), Y1
	VPMAXSD Y2, Y1, Y1
	VPMINSD Y3, Y1, Y1
	VPMULLD Y1, Y0, Y0
	VPSRLD $7, Y0, Y0
	VEXTRACTI128 $1, Y0, X1
	VPSHUFB X4, X0, X0
	VPSHUFB X4, X1, X1
	VPSLLDQ $4, X1, X1
	VPOR X1, X0, X0
	VMOVQ X0, (BX)(R9*1)
	ADDQ $8, R9
	CMPQ R9, $512
	JLT transformLoop
	INCQ R8
	CMPQ R8, $2
	JLT perspLoop
	VZEROUPPER
	RET

// func addFeatureAVX2(values *int32, add *int16, n int)
TEXT ·addFeatureAVX2(SB), NOSPLIT, $0-24
	MOVQ values+0(FP), DI
	MOVQ add+8(FP), SI
	MOVQ n+16(FP), CX
	XORQ AX, AX
addLoop:
	VPMOVSXWD (SI)(AX*2), Y0
	VPADDD (DI)(AX*4), Y0, Y0
	VMOVDQU Y0, (DI)(AX*4)
	ADDQ $8, AX
	CMPQ AX, CX
	JLT addLoop
	VZEROUPPER
	RET

// func applyDelta2AVX2(values *int32, add, remove1, remove2 *int16, n int)
TEXT ·applyDelta2AVX2(SB), NOSPLIT, $0-40
	MOVQ values+0(FP), DI
	MOVQ add+8(FP), SI
	MOVQ remove1+16(FP), BX
	MOVQ remove2+24(FP), R8
	MOVQ n+32(FP), CX
	XORQ AX, AX
delta2Loop:
	VPMOVSXWD (SI)(AX*2), Y0
	VPMOVSXWD (BX)(AX*2), Y1
	VPSUBD Y1, Y0, Y0
	VPMOVSXWD (R8)(AX*2), Y1
	VPSUBD Y1, Y0, Y0
	VPADDD (DI)(AX*4), Y0, Y0
	VMOVDQU Y0, (DI)(AX*4)
	ADDQ $8, AX
	CMPQ AX, CX
	JLT delta2Loop
	VZEROUPPER
	RET
