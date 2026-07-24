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
