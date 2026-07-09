	.section .text.start,"ax"
	.global _start
_start:
	mov.l	stack_top_addr, r15     ! set SP
	! zero .bss
	mov.l	bss_start_addr, r0
	mov.l	bss_end_addr, r1
1:	cmp/hs	r1, r0
	bt	2f
	mov	#0, r2
	mov.l	r2, @r0
	add	#4, r0
	bra	1b
	nop
2:	mov.l	main_addr, r0
	jsr	@r0
	nop
3:	bra	3b                       ! main returns should not happen
	nop
	.align 2
stack_top_addr:	.long __stack_top
bss_start_addr:	.long __bss_start
bss_end_addr:	.long __bss_end
! _main = asm name of CoreMark's C main() under this toolchain's leading-underscore convention
main_addr:	.long _main
