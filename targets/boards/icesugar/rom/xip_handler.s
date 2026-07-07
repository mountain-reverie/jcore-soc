! xip_handler.s -- resident Page-Fault handler for the iCESugar XIP page
! cache (spi_page_cache, components/misc/spi_page_cache.vhd). MUST be
! resident (SPRAM/EBR), NOT in the demand-paged flash window, since the
! handler cannot be allowed to fault while it is handling a fault.
!
! Dispatch convention (established elsewhere in this repo -- see
! components/cpu/sim/tests/pagefault_i.S and start.S's _aic_isr comment):
! the base J1 (no MMU/PRIV) vectors ALL exceptions/interrupts through a
! single entry point at mem[VBR+0]. This handler is that entry, SHARED
! with illegal-instruction: it first checks the page-cache STATUS register
! (0xABCD0414) for `pending` (bit0); if set, this is a page fault and we
! service it below. If clear, this dispatch was NOT a page fault (e.g. an
! illegal instruction or the AIC's interrupt) and we fall through to
! `_illegal_panic` (a placeholder -- integrating this with a real AIC/illegal
! dispatcher is out of scope for this task; see the report for what a future
! integration must wire).
!
! Page-cache MMIO map (base 0xABCD0400, from the Task-2/3 briefs):
!   +0x00..0x0C  TAG0..TAG3   R/W  {valid:bit8, page:bits7:0}
!   +0x10        FAULT_VA     R    faulting virtual address
!   +0x14        STATUS       R: {pending:bit0, last_kind:bit1}
!                              W: write 1 to bit0 clears pending
!   +0x18        FILL_CMD     W    {frame:bits9:8, page:bits7:0}; write pulses
!                                  the embedded spi_flash_fill engine
!   +0x1C        FILL_STATUS  R    {busy:bit0, done:bit1}
!
! Page-in flow (HW-assisted fill -- no byte-copy loop, the fill engine
! streams the 4 KB flash page straight into the victim frame's EBR):
!   1. save r0-r7/PR (SH-2 has no banked registers; the interrupted code's
!      registers must be preserved exactly).
!   2. page = (FAULT_VA >> 12) & 0xFF   (extu.b performs the & 0xFF)
!   3. victim = (round-robin counter++) & 3
!   4. invalidate TAG[victim] (write 0 -> valid bit clear) BEFORE the fill,
!      so a spurious hit against the stale mapping cannot occur mid-fill.
!   5. FILL_CMD = (victim<<8)|page; poll FILL_STATUS.done (bit1).
!   6. TAG[victim] = (1<<8)|page (valid + page).
!   7. STATUS write with bit0=1 clears `pending`.
!   8. restore r0-r7/PR; RTE (re-fetches/re-executes the faulting access).

! plain .s (no C preprocessor pass), so use gas .equ instead of #define.
	.equ	PC_TAG_BASE,    0xABCD0400
	.equ	PC_FAULT_VA,    0xABCD0410
	.equ	PC_STATUS,      0xABCD0414
	.equ	PC_FILL_CMD,    0xABCD0418
	.equ	PC_FILL_STATUS, 0xABCD041C

! ---- reset vectors + crt0 (resident; xip.x places these in the boot EBR at
! 0x0, same as targets/boards/icesugar/rom/icesugar.ld's convention: word0 =
! reset PC, word1 = reset SP). ----
	.section .vectors, "ax"
	.global _vectors
_vectors:
	.long _start
	.long __stack_top
	.long _start
	.long __stack_top

	.section .boot.text, "ax"
	.align 2
	.global _start
_start:
	mov.l	sp_init, r15
	! zero .bss (SPRAM)
	mov.l	bss_start, r1
	mov.l	bss_end, r2
	mov	#0, r0
2:	cmp/hs	r2, r1
	bt	3f
	mov.l	r0, @r1
	add	#4, r1
	bra	2b
	nop
	! VBR -> _vbr_table, so mem[VBR+0] = _pf_handler (shared page-fault /
	! illegal-instruction dispatch, per the header comment above).
3:	mov.l	vbr_table_addr, r0
	ldc	r0, vbr
	mov.l	main_addr, r0
	jsr	@r0
	nop
1:	bra	1b
	nop
	.align 2
sp_init:         .long __stack_top
main_addr:       .long _main
bss_start:       .long __bss_start
bss_end:         .long __bss_end
vbr_table_addr:  .long _vbr_table

	.align 2
	.global _vbr_table
_vbr_table:
	.long _pf_handler	! VBR+0: the single shared exception entry point

	.align 2
	.global _pf_handler
_pf_handler:
	mov.l	r0, @-r15
	mov.l	r1, @-r15
	mov.l	p_status, r0
	mov.l	@r0, r1
	mov	r1, r0
	tst	#1, r0			! T=1 if (STATUS & 1)==0 -> not pending
	bt	_pf_not_pending

	! ---- page-fault path: save the remaining caller-visible state ----
	mov.l	r2, @-r15
	mov.l	r3, @-r15
	mov.l	r4, @-r15
	mov.l	r5, @-r15
	mov.l	r6, @-r15
	mov.l	r7, @-r15
	sts.l	pr, @-r15

	! page = (FAULT_VA >> 12) & 0xFF
	mov.l	p_fault_va, r0
	mov.l	@r0, r1
	shlr8	r1
	shlr2	r1
	shlr2	r1			! r1 = FAULT_VA >> 12
	extu.b	r1, r4			! r4 = page (zero-extended low byte)

	! victim = (counter++) & 3
	mov.l	p_victim_ctr, r0
	mov.l	@r0, r5
	mov	r5, r1
	add	#1, r1
	mov.l	r1, @r0
	mov	r5, r0
	and	#3, r0
	mov	r0, r6			! r6 = victim (0..3)

	! invalidate TAG[victim] before the fill
	mov.l	p_tag_base, r0
	mov	r6, r1
	shll2	r1
	add	r1, r0
	mov	#0, r1
	mov.l	r1, @r0

	! FILL_CMD = (victim << 8) | page
	mov	r6, r2
	shll8	r2
	or	r4, r2
	mov.l	p_fill_cmd, r0
	mov.l	r2, @r0

	! poll FILL_STATUS.done (bit1)
_pf_poll:
	mov.l	p_fill_status, r0
	mov.l	@r0, r1
	mov	r1, r0
	tst	#2, r0
	bt	_pf_poll

	! TAG[victim] = valid(bit8) | page
	mov	#1, r2
	shll8	r2
	or	r4, r2
	mov.l	p_tag_base, r0
	mov	r6, r1
	shll2	r1
	add	r1, r0
	mov.l	r2, @r0

	! clear STATUS.pending
	mov.l	p_status, r0
	mov	#1, r1
	mov.l	r1, @r0

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

_pf_not_pending:
	mov.l	@r15+, r1
	mov.l	@r15+, r0
	bra	_illegal_panic
	nop

	.align 2
p_status:      .long PC_STATUS
p_fault_va:    .long PC_FAULT_VA
p_fill_cmd:    .long PC_FILL_CMD
p_fill_status: .long PC_FILL_STATUS
p_tag_base:    .long PC_TAG_BASE
p_victim_ctr:  .long _pf_victim_ctr

! Round-robin victim-frame counter. Resident data (SPRAM), NOT paged.
	.section .boot.data, "aw"
	.align 2
	.global _pf_victim_ctr
_pf_victim_ctr: .long 0

! Placeholder for the shared-dispatch "not a page fault" path (illegal
! instruction / other trap). No real illegal-instruction handler exists yet
! for this standalone test image; a future integration (full SoC dispatcher)
! must replace this stub. Spins so a misrouted dispatch is observable (rather
! than silently corrupting state) instead of falling into undefined code.
	.section .boot.text, "ax"
	.align 2
	.global _illegal_panic
_illegal_panic:
1:	bra	1b
	nop
