OUTPUT_FORMAT("elf32-sh")
OUTPUT_ARCH(sh)
ENTRY(_start)
/* All memory is on-chip inferred EBR (bootram_infer, c_addr_width = 11 ->
   2 KiB = 512 32-bit words), mapped at the SuperH reset vector 0x00000000
   (DEV_SRAM). Code + data + stack all live here; there is no SDRAM. */
MEMORY {
  BOOT (rwx) : ORIGIN = 0x00000000, LENGTH = 2K
}
SECTIONS {
  .vectors 0x0 : { KEEP(*(.vectors)) } > BOOT
  .text   : { *(.text) *(.text.*) } > BOOT
  .rodata : { *(.rodata) *(.rodata.*) } > BOOT
  .data   : { *(.data) *(.data.*) } > BOOT
  . = ALIGN(4); __bss_start = .;
  .bss (NOLOAD) : { *(.bss) *(.bss.*) *(COMMON) } > BOOT
  . = ALIGN(4); __bss_end = .;
  /DISCARD/ : { *(.comment) *(.note*) }
}
