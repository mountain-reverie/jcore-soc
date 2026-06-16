OUTPUT_FORMAT("elf32-sh")
OUTPUT_ARCH(sh)
ENTRY(_start)
MEMORY {
  BOOT (rwx)       : ORIGIN = 0x00000000, LENGTH = 16K
  BOOT_SDRAM (rwx) : ORIGIN = 0x10000000, LENGTH = 1M
}
SECTIONS {
  .vectors 0x0 : { KEEP(*(.vectors)) } > BOOT
  .text   : { *(.text) *(.text.*) } > BOOT
  .ivt    : ALIGN(4) { KEEP(*(.ivt)) } > BOOT   /* VBR interrupt vector table */
  .rodata : { *(.rodata) *(.rodata.*) } > BOOT
  .data   : { *(.data) *(.data.*) } > BOOT
  . = ALIGN(4); __bss_start = .;
  .bss (NOLOAD) : { *(.bss) *(.bss.*) *(COMMON) } > BOOT
  . = ALIGN(4); __bss_end = .;
  /* Runs from SDRAM (VMA 0x10000000) but is stored (LMA) in BRAM so the boot
     program can copy it up before jumping. */
  .sdram : ALIGN(4) {
    _sdram_start = .;
    *(.sdram .sdram.*)
    . = ALIGN(4);
    _sdram_end = .;
  } > BOOT_SDRAM AT> BOOT
  _sdram_load = LOADADDR(.sdram);
  /DISCARD/ : { *(.comment) *(.note*) }
}
