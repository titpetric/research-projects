TEXT app.BenchmarkStringJoin2B(SB) /go/src/app/recursion_test.go
	FS MOVQ FS:0xfffffff8, CX
	CMPQ 0x10(CX), SP
	JBE 0x46e63c
	SUBQ $0x80, SP
	MOVQ BP, 0x78(SP)
	LEAQ 0x78(SP), BP
	XORPS X0, X0
	MOVUPS X0, 0x58(SP)
	MOVQ 0x88(SP), AX
	MOVB $0x1, 0xea(AX)
	XORL CX, CX
	MOVQ CX, 0x40(SP)
	MOVQ 0xb8(AX), DX
	CMPQ DX, CX
	JGE 0x46e5f2
	LEAQ 0x7b257(IP), DX
	MOVQ DX, 0(SP)
	CALL runtime.newobject(SB)
	MOVQ 0x8(SP), DI
	MOVQ DI, 0x50(SP)
	TESTB AL, 0(DI)
	LEAQ 0xaf73b(IP), SI
	MOVQ BP, -0x10(SP)
	LEAQ -0x10(SP), BP
	CALL 0x457c44
	MOVQ 0(BP), BP
	MOVQ 0x50(SP), AX
	MOVQ AX, 0(SP)
	MOVQ $0x2, 0x8(SP)
	MOVQ $0x2, 0x10(SP)
	LEAQ 0x9fab6(IP), AX
	MOVQ AX, 0x18(SP)
	MOVQ $0x1, 0x20(SP)
	MOVQ 0xaecf9(IP), AX
	LEAQ 0xaecf2(IP), DX
	CALL AX
	MOVQ 0x28(SP), AX
	MOVQ AX, 0x48(SP)
	MOVQ 0x30(SP), CX
	MOVQ CX, 0x38(SP)
	CMPQ $0xb, CX
	JE 0x46e5ff
	MOVQ $0x0, 0(SP)
	LEAQ 0xa246d(IP), DX
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
	LEAQ 0x78391(IP), AX
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
	MOVQ 0x40(SP), DX
	LEAQ 0x1(DX), CX
	MOVQ 0x88(SP), AX
	MOVQ CX, 0x40(SP)
	MOVQ 0xb8(AX), DX
	CMPQ DX, CX
	JL 0x46e482
	MOVQ 0x78(SP), BP
	ADDQ $0x80, SP
	RET
	MOVQ AX, 0(SP)
	MOVQ CX, 0x8(SP)
	LEAQ 0xa0ca4(IP), DX
	MOVQ DX, 0x10(SP)
	MOVQ $0xb, 0x18(SP)
	CALL runtime.eqstring(SB)
	MOVZX 0x20(SP), AX
	TESTL AL, AL
	JE 0x46e62d
	JMP 0x46e5cc
	MOVQ 0x48(SP), AX
	MOVQ 0x38(SP), CX
	JMP 0x46e516
	CALL runtime.morestack_noctxt(SB)
	JMP app.BenchmarkStringJoin2B(SB)
