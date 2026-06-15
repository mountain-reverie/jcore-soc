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
	mov.l	main_addr, r0
	jsr	@r0
	nop
1:	bra	1b
	nop
	.align 2
sp_init:    .long 0x00003ffc
main_addr:  .long _main
