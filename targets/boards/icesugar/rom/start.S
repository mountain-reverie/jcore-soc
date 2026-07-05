	! SH-2 reset: word 0 = initial PC, word 1 = initial SP. All memory is
	! on-chip EBR (2 KiB) at 0x0; the stack starts at the top of it.
	.section .vectors, "ax"
	.global _vectors
_vectors:
	.long _start          ! 0x00: power-on PC
	.long 0x000007fc      ! 0x04: power-on SP (top of 2 KiB boot RAM)
	.long _start          ! 0x08: manual-reset PC
	.long 0x000007fc      ! 0x0c: manual-reset SP

! AIC interrupt vector table (VBR-relative). Entries are fetched at
! VBR + vec*4, where vec is the 8-bit vector number the CPU gets from the
! AIC's event_i.vec (components/misc/aic.vhd's vector_numbers(N) for
! irq_i(N)). The iCESugar aic0 instance (targets/boards/icesugar/design.yaml)
! does not override vector_numbers, so it keeps aic.vhd's default
! (x"11",x"12",...,x"18") -- irq_i(1), the DS3231 SQW tick wired by
! ice_irq_in, is vector 0x12 = 18 decimal, i.e. table index 18. Entries 0..17
! are never triggered on this board (only irq_i(1) is wired to a real
! interrupt source) but the table must still be contiguous up to index 18;
! point the unused entries at a harmless rte stub in case of a spurious
! event.
	.section .rodata
	.align 2
	.global _int_vector_table
! The J-core CPU vectors ALL maskable interrupts through a single entry at
! VBR+0 (empirically confirmed: the AIC's event drives the interrupt microcode
! which reads the handler from mem[VBR+0], NOT SH-1/2 auto-vectoring by
! event_i.vec). So _aic_isr is index 0; on this board only irq_i(1) (the DS3231
! SQW tick) is a live source, so index 0 IS the tick ISR. The remaining entries
! are a harmless rte stub, kept only so an errant read near VBR lands safely.
_int_vector_table:
	.long _aic_isr         ! 0: the single interrupt entry point (VBR+0)
	.irp idx, 1,2,3,4,5,6,7,8,9,10,11,12,13,14,15
	.long _spurious_isr
	.endr

	.section .text
	.global _start
	.align 2
_start:
	mov.l	sp_init, r15
	! zero .bss so writable statics start at 0
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
sp_init:    .long 0x000007fc
main_addr:  .long _main
bss_start:  .long __bss_start
bss_end:    .long __bss_end

! Spurious/unexpected interrupt: just return. Only reachable if an errant
! event vectors to a non-zero table index (should not happen on this board).
	.align 2
	.global _spurious_isr
_spurious_isr:
	rte
	nop

! Interrupt ISR (single entry, VBR+0): the only live source on this board is
! aic0 irq_i(1), the DS3231 SQW tick. Bump the observable tick counter and
! return. CRITICAL: an SH-2 ISR shares r0-r15 with the interrupted code (no
! banked registers), so it MUST save/restore every register it touches, or it
! corrupts whatever was running (this was the original interrupt-hang bug). The
! AIC edge-detect latch is cleared automatically by the CPU's event_i.ack
! handshake during interrupt entry -- no explicit aic write needed to re-arm.
	.align 2
	.global _aic_isr
_aic_isr:
	mov.l	r0, @-r15		! save r0
	mov.l	r1, @-r15		! save r1
	mov.l	irq_tick_count_addr, r0
	mov.l	@r0, r1
	add	#1, r1
	mov.l	r1, @r0
	mov.l	@r15+, r1		! restore r1
	mov.l	@r15+, r0		! restore r0
	rte
	nop
	.align 2
irq_tick_count_addr: .long _irq_tick_count

! Program VBR to the interrupt vector table above and lower SR's interrupt
! mask (I3-I0, reset value 0xF = all maskable interrupts blocked) to 0 so
! any AIC-issued interrupt level (1-15) satisfies the CPU's acceptance test
! ibit < event_i.lvl (components/cpu/decode/decode_core.vhm). Called once
! from banner.c's main() after aic0's ilevel(1) has been programmed.
	.align 2
	.global _enable_interrupts
_enable_interrupts:
	mov.l	vbr_table_addr, r0
	ldc	r0, vbr
	stc	sr, r0
	mov.l	sr_imask_clear, r1
	and	r1, r0
	ldc	r0, sr
	rts
	nop
	.align 2
vbr_table_addr: .long _int_vector_table
sr_imask_clear: .long 0xffffff0f
