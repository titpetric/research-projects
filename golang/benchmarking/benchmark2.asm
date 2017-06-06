TEXT app.BenchmarkStringJoin2(SB) /go/src/app/recursion_test.go
	FS MOVQ FS:0xfffffff8, CX
	CMPQ 0x10(CX), SP
	JBE 0x46e41c
	SUBQ $0x80, SP
	MOVQ BP, 0x78(SP)
	LEAQ 0x78(SP), BP
	XORPS X0, X0
	MOVUPS X0, 0x58(SP)
	MOVQ 0x88(SP), AX
	MOVB $0x1, 0xea(AX)
	LEAQ 0x7b47e(IP), CX
	MOVQ CX, 0(SP)
	CALL runtime.newobject(SB)
	MOVQ 0x8(SP), DI
	MOVQ DI, 0x50(SP)
	TESTB AL, 0(DI)
	LEAQ 0xaf942(IP), SI
	MOVQ BP, -0x10(SP)
	LEAQ -0x10(SP), BP
	CALL 0x457c44
	MOVQ 0(BP), BP
	XORL AX, AX
	MOVQ AX, 0x40(SP)
	MOVQ 0x88(SP), CX
	MOVQ 0xb8(CX), BX
	CMPQ BX, AX
	JGE 0x46e40f
	MOVQ 0x50(SP), BX
	MOVQ BX, 0(SP)
	MOVQ $0x2, 0x8(SP)
	MOVQ $0x2, 0x10(SP)
	LEAQ 0x9fcbe(IP), SI
	MOVQ SI, 0x18(SP)
	MOVQ $0x1, 0x20(SP)
	MOVQ 0xaeef9(IP), SI
	LEAQ 0xaeef2(IP), DX
	CALL SI
	MOVQ 0x28(SP), AX
	MOVQ AX, 0x48(SP)
	MOVQ 0x30(SP), CX
	MOVQ CX, 0x38(SP)
	CMPQ $0xb, CX
	JE 0x46e3d2
	MOVQ $0x0, 0(SP)
	LEAQ 0xa2675(IP), DX
	MOVQ DX, 0x8(SP)
	MOVQ $0x13, 0x10(SP)
	MOVQ AX, 0x18(SP)
	MOVQ CX, 0x20(SP)
	CALL runtime.concatstring2(SB)
	MOVQ 0x28(SP), AX
	MOVQ 0x30(SP), CX
	MOVQ AX, 0x68(SP)
	MOVQ CX, 0x70(SP)
	MOVQ $0x0, 0x58(SP)
	MOVQ $0x0, 0x60(SP)
	LEAQ 0x78599(IP), AX
	MOVQ AX, 0(SP)
	LEAQ 0x68(SP), AX
	MOVQ AX, 0x8(SP)
	MOVQ $0x0, 0x10(SP)
	CALL runtime.convT2E(SB)
	MOVQ 0x20(SP), AX
	MOVQ 0x18(SP), CX
	MOVQ CX, 0x58(SP)
	MOVQ AX, 0x60(SP)
	MOVQ 0x88(SP), AX
	MOVQ AX, 0(SP)
	LEAQ 0x58(SP), CX
	MOVQ CX, 0x8(SP)
	MOVQ $0x1, 0x10(SP)
	MOVQ $0x1, 0x18(SP)
	CALL testing.(*common).Error(SB)
	MOVQ 0x40(SP), CX
	LEAQ 0x1(CX), AX
	JMP 0x46e293
	MOVQ AX, 0(SP)
	MOVQ CX, 0x8(SP)
	LEAQ 0xa0ed1(IP), DX
	MOVQ DX, 0x10(SP)
	MOVQ $0xb, 0x18(SP)
	CALL runtime.eqstring(SB)
	MOVZX 0x20(SP), AX
	TESTL AL, AL
	JE 0x46e400
	JMP 0x46e3c4
	MOVQ 0x48(SP), AX
	MOVQ 0x38(SP), CX
	JMP 0x46e30e
	MOVQ 0x78(SP), BP
	ADDQ $0x80, SP
	RET
	CALL runtime.morestack_noctxt(SB)
	JMP app.BenchmarkStringJoin2(SB)
