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
	! zero .bss so writable statics (e.g. g_ticks) start at 0
	mov.l	bss_start, r1
	mov.l	bss_end, r2
	mov	#0, r0
2:	cmp/hs	r2, r1          ! T if r1 >= r2 (done)
	bt	3f
	mov.l	r0, @r1
	add	#4, r1
	bra	2b
	nop
3:	! M2: point VBR at the interrupt vector table, then enable interrupts
	mov.l	vbr_base, r0
	ldc	r0, vbr
	mov	#0, r0          ! SR = 0: I3-I0 = 0 (accept all levels); T/S/M/Q = 0
	ldc	r0, sr          ! (PIT/GPIO are still disabled in the AIC until C arms)
	mov.l	main_addr, r0
	jsr	@r0
	nop
1:	bra	1b
	nop
	.align 2
sp_init:    .long 0x00003ffc
main_addr:  .long _main
bss_start:  .long __bss_start
bss_end:    .long __bss_end
vbr_base:   .long _vector_table

	! Interrupt trampolines: two sources distinguished by VECTOR (no post-ack
	! status race) -- PIT (0x40) -> irq_dispatch, GPIO button (0x11) -> irq_gpio.
	! Each entry saves r0, loads its C handler into r0, and joins the common
	! save/call/restore/RTE. On entry the SH-2 has already pushed SR and PC; RTE
	! pops PC then SR.
	.align 2
	.global _irq_pit_entry
_irq_pit_entry:
	mov.l	r0, @-r15
	mov.l	pit_disp, r0
	bra	_irq_common
	nop
	.align 2
	.global _irq_gpio_entry
_irq_gpio_entry:
	mov.l	r0, @-r15
	mov.l	gpio_disp, r0
	bra	_irq_common
	nop
_irq_common:                    ! r0 = C handler addr; caller's r0 already saved
	mov.l	r1, @-r15
	mov.l	r2, @-r15
	mov.l	r3, @-r15
	mov.l	r4, @-r15
	mov.l	r5, @-r15
	mov.l	r6, @-r15
	mov.l	r7, @-r15
	sts.l	pr, @-r15
	sts.l	mach, @-r15
	sts.l	macl, @-r15
	jsr	@r0
	nop
	lds.l	@r15+, macl
	lds.l	@r15+, mach
	lds.l	@r15+, pr
	mov.l	@r15+, r7
	mov.l	@r15+, r6
	mov.l	@r15+, r5
	mov.l	@r15+, r4
	mov.l	@r15+, r3
	mov.l	@r15+, r2
	mov.l	@r15+, r1
	mov.l	@r15+, r0
	rte
	nop
	.align 2
pit_disp:   .long _irq_dispatch
gpio_disp:  .long _irq_gpio

	! Fault catch-all for CPU exception vectors (illegal instruction, slot
	! illegal, address error, ...). Halts visibly instead of letting an
	! unexpected fault loop through the IRQ path and silently inflate g_ticks.
	.align 2
	.global _fault_entry
_fault_entry:
	bra	_fault_entry
	nop

	! VBR-relative vector table (4 bytes/entry, indexed by vector number). VBR is
	! set to _vector_table so an exception/interrupt with vector N fetches its
	! handler from _vector_table + N*4. CPU-exception slots halt at _fault_entry;
	! the GPIO button IRQ (0x11 = vector_numbers(0)) and the PIT (0x40) run their
	! trampolines. (Reset itself uses the 0x0 .vectors, not VBR.)
	.section .ivt, "a"
	.align 2
	.global _vector_table
_vector_table:
	.rept 0x11
	.long _fault_entry      ! 0x00..0x10
	.endr
	.long _irq_gpio_entry   ! 0x11: GPIO button (aic vector_numbers(0))
	.rept (0x40 - 0x12)
	.long _fault_entry      ! 0x12..0x3F
	.endr
	.long _irq_pit_entry    ! 0x40: PIT timer interrupt
