! Task 4 (QSPI XIP cosim) minimal SH-2 payload -- SCRATCHPAD REMOVAL variant.
!
! Linked to run AT 0x14000000 (the flash XIP entry, see
! boot_image_pkg.vhd's vector table: word0=PC=0x14000000,
! word1=SP=0x10001000). Reset loads PC/SP DIRECTLY from the boot ROM
! vector table (bootram_infer(boot_mem), now pure read-only ROM -- the
! on-chip writable boot scratchpad has been ELIMINATED, see
! components/memory/boot_mem.vhd), then the CPU's first FETCH is at
! 0x14000000 -> DEV_DDR -> mem_region_mux -> qspi_flash_ctrl.
!
! SP (r15) is already 0x10001000 -- an SDRAM address (SDRAM base
! 0x10000000, DEV_DDR) -- straight from the reset vector; this payload
! never touches r15 to set it up. That is the point of this proof:
! components/sdram/sdram_ctrl.vhd self-initialises in hardware (its FSM
! runs the full SDRAM init sequence automatically from reset and only
! serves accesses once idle), so the FIRST SDRAM access -- the `@-r15`
! push below -- simply STALLS in hardware until init completes. No
! software SDRAM-init/wait code is required or present here.
!
! Effect (TB-OBSERVABLE): PUSH a 32-bit signature (0xF1A5B007) onto the
! SDRAM stack via `mov.l r0,@-r15` (pre-decrement store -- genuine SH-2
! stack-push semantics), landing at SDRAM byte address 0x10000FFC (SP-4),
! then spin forever. The signature can only appear there if (a) this code
! was actually FETCHED and EXECUTED from flash at 0x14000000, AND (b) the
! SDRAM stack pointer loaded from the reset vector actually works as a
! stack with no on-chip scratchpad backing it -- there is no other path
! to that store.
!
! r0 (the signature) is loaded via a PC-relative literal-pool read
! (`mov.l @(disp,PC),Rn`) -- a genuine flash-served DATA read (dcache
! miss through qspi_flash_ctrl), not just an instruction fetch (icache).
! This still exercises both halves of XIP: execute-in-place (icache) and
! data-read-in-place (dcache) -- see payload's prior HISTORY note (kept
! below) for why the literal-pool form matters.
!
! HISTORY (do not "simplify" this back to immediate-built operands): an
! earlier version of this exact payload hung -- the store never issued --
! and was traced (see .superpowers/sdd/task-4-report.md, "Fix:
! mem_region_mux response-path" / register-banking debug sections) to a
! qspi_flash_ctrl bug, now fixed: its burst-streaming logic always started
! delivering the cache line at word 0, regardless of which word was
! actually requested, instead of critical-word-first. Fixed in
! components/misc/qspi_flash_ctrl.vhd (bst_widx). Keep the literal-pool
! read for the signature value.
	.section .text
	.global _start
	.align 2
_start:
	mov.l	sig_val, r0
	mov.l	r0, @-r15
spin:
	bra	spin
	nop
	.align 4
sig_val:
	.long	0xf1a5b007
