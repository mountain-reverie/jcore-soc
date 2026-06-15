	! SH-2 reset: word 0 = initial PC, word 1 = initial SP
	.section .vectors, "ax"
	.global _vectors
_vectors:
	.long _start          ! 0x00: power-on PC
	.long 0x00003ffc      ! 0x04: power-on SP (top of 16 KiB boot RAM)
	.long _start          ! 0x08: manual-reset PC
	.long 0x00003ffc      ! 0x0c: manual-reset SP

	.section .text
	.global _start
	.align 2
_start:
	mov.l	sp_init, r15
	! zero .bss so writable statics are defined (empty today, safe for growth)
	mov.l	bss_start, r1
	mov.l	bss_end, r2
	mov	#0, r0
2:	cmp/hs	r2, r1          ! T if r1 >= r2 (done)
	bt	3f
	mov.l	r0, @r1
	add	#4, r1
	bra	2b
	nop
3:	mov.l	main_addr, r0
	jsr	@r0
	nop
1:	bra	1b
	nop
	.align 2
sp_init:    .long 0x00003ffc
main_addr:  .long _main
bss_start:  .long __bss_start
bss_end:    .long __bss_end
