OUTPUT_FORMAT("elf32-sh")
OUTPUT_ARCH(sh)
ENTRY(_start)
/* All memory is on-chip inferred EBR (bootram_infer, c_addr_width = 11 ->
   2 KiB = 512 32-bit words), mapped at the SuperH reset vector 0x00000000
   (DEV_SRAM). Code + data + stack all live here; there is no SDRAM. */
MEMORY {
  BOOT       (rwx) : ORIGIN = 0x00000000, LENGTH = 2K
  BOOT_SPRAM (rwx) : ORIGIN = 0x10000000, LENGTH = 128K
}
SECTIONS {
  .vectors 0x0 : { KEEP(*(.vectors)) } > BOOT
  .text   : { *(.text) *(.text.*) } > BOOT
  .rodata : { *(.rodata) *(.rodata.*) } > BOOT
  .data   : { *(.data) *(.data.*) } > BOOT
  . = ALIGN(4); __bss_start = .;
  .bss (NOLOAD) : { *(.bss) *(.bss.*) *(COMMON) } > BOOT
  . = ALIGN(4); __bss_end = .;

  /* Small routine that RUNS from SPRAM: VMA in SPRAM, LMA stored in EBR so the
     boot code can copy it up before jumping. Keep it small — the LMA competes
     with .text/.data in the 2K EBR. */
  .spram 0x10000000 : ALIGN(4) {
    __spram_start = .;
    *(.spram .spram.*)
    . = ALIGN(4);
    __spram_end = .;
  } > BOOT_SPRAM AT> BOOT
  __spram_load = LOADADDR(.spram);

  /DISCARD/ : { *(.comment) *(.note*) }
}
