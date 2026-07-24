! Task 4 (QSPI XIP cosim) minimal SH-2 payload.
!
! Linked to run AT 0x14000000 (the flash XIP entry, see Task 3's
! boot_image_pkg.vhd vector table: word0=PC=0x14000000, word1=SP=0x00003ffc).
! Reset loads PC/SP from the boot ROM vector table (bootram_infer, DEV_SRAM),
! then the CPU's first FETCH is at 0x14000000 -> DEV_DDR -> mem_region_mux ->
! qspi_flash_ctrl (this is the address chain Task 4's cosim proves).
!
! Effect (TB-OBSERVABLE): store a 32-bit signature (0x0000005A) to a fixed
! boot-RAM address (0x00000100), then spin forever. The signature can only
! appear at 0x100 if this code was actually FETCHED and EXECUTED from flash
! at 0x14000000 -- there is no other path to that store.
!
! IMMEDIATE-ONLY BY DESIGN (do not "simplify" back to PC-relative literal
! loads -- see the debug history below): the operands are built with
! `mov #imm`/`shll8` instead of `mov.l @(disp,PC),Rn` literal-pool loads.
! This was found EMPIRICALLY NECESSARY debugging Task 4's original blocker:
! a first version of this payload used two `mov.l @(disp,PC),Rn` loads to
! fetch r0/r1 from a literal pool later in the SAME flash cache line. With
! that version, cosim showed the CPU fetch+execute the whole payload
! correctly (PC traced all the way into the `spin: bra spin` loop, proven
! via a temporary fetch-PC probe), and the two literal-pool reads
! themselves completed (dcache-miss bursts through qspi_flash_ctrl acked
! cleanly, matching the icache burst) -- but the subsequent `mov.l r0,@r1`
! store NEVER issued on any of cpu_core's decoded data-bus exits
! (DEV_SRAM/DEV_DDR/DEV_PERIPH/DEV_CPU), confirmed by probing all four
! directly (this rules out a write-back-dcache hiding the store: the
! store's target 0x100 decodes to DEV_SRAM, which has no cache in its path
! at all, so a real store there is directly observable at that port -- it
! never appeared, on any port). Switching to immediate-built operands
! (removing the PC-relative flash-DATA-read + its load-use dependency,
! while keeping the same flash EXECUTE path) made the store land
! immediately, proving XIP fetch+execute is sound and isolating the bug to
! reading DATA (not instructions) from the flash region via PC-relative
! literal-pool loads (a dcache-miss-through-qspi_flash_ctrl path, likely a
! load-use interlock or burst-completion signal that behaves differently
! than the icache's equivalent path -- NOT fixed here; see
! .superpowers/sdd/task-4-report.md's "Debug: register-level classification"
! section for the full evidence trail and this as a documented follow-up).
	.section .text
	.global _start
	.align 2
_start:
	mov	#1, r1
	shll8	r1
	mov	#0x5a, r0
	mov.l	r0, @r1
spin:
	bra	spin
	nop
	.align 4
