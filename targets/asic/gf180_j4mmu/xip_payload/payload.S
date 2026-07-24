! Task 4 (QSPI XIP cosim) minimal SH-2 payload.
!
! Linked to run AT 0x14000000 (the flash XIP entry, see Task 3's
! boot_image_pkg.vhd vector table: word0=PC=0x14000000, word1=SP=0x00003ffc).
! Reset loads PC/SP from the boot ROM vector table (bootram_infer, DEV_SRAM),
! then the CPU's first FETCH is at 0x14000000 -> DEV_DDR -> mem_region_mux ->
! qspi_flash_ctrl (this is the address chain Task 4's cosim proves).
!
! Effect (TB-OBSERVABLE): store a 32-bit signature (0xF1A5B007) to a fixed
! boot-RAM address (0x00000100), then spin forever. The signature can only
! appear at 0x100 if this code was actually FETCHED and EXECUTED from flash
! at 0x14000000 -- there is no other path to that store.
!
! r0/r1 are loaded via PC-relative literal-pool reads (`mov.l @(disp,PC),Rn`)
! -- genuine flash-served DATA reads (dcache misses through qspi_flash_ctrl),
! not just instruction fetches (icache). This exercises BOTH halves of XIP:
! execute-in-place (icache) and data-read-in-place (dcache).
!
! HISTORY (do not "simplify" this back to immediate-built operands): an
! earlier version of this exact payload hung -- the store never issued --
! and was traced (see .superpowers/sdd/task-4-report.md, "Fix:
! mem_region_mux response-path" / register-banking debug sections) to a
! qspi_flash_ctrl bug, now fixed: its burst-streaming logic always started
! delivering the cache line at word 0, regardless of which word was
! actually requested, instead of critical-word-first (deliver the
! REQUESTED word first, then wrap through the rest of the line -- the
! order sdram_ctrl/real SDRAM burst-wrap semantics use, and the order the
! CPU's dcache_mcl assumes). Since sig_val/sig_addr are NOT at word 0/1 of
! their cache line, the dcache silently captured the WRONG word (word 0/1
! of the line -- coincidentally the payload's own first instruction bytes)
! as if it were the requested literal, corrupting r0/r1. Fixed in
! components/misc/qspi_flash_ctrl.vhd (bst_widx). This payload is the
! actual regression proof for that fix -- keep it literal-pool-based.
	.section .text
	.global _start
	.align 2
_start:
	mov.l	sig_val, r0
	mov.l	sig_addr, r1
	mov.l	r0, @r1
spin:
	bra	spin
	nop
	.align 4
sig_val:
	.long	0xf1a5b007
sig_addr:
	.long	0x00000100
