OUTPUT_FORMAT("elf32-sh")
OUTPUT_ARCH(sh)
ENTRY(_start)
MEMORY { BOOT (rwx) : ORIGIN = 0x00000000, LENGTH = 16K }
SECTIONS {
  .vectors 0x0 : { KEEP(*(.vectors)) } > BOOT
  .text   : { *(.text) *(.text.*) } > BOOT
  .rodata : { *(.rodata) *(.rodata.*) } > BOOT
  .data   : { *(.data) *(.data.*) } > BOOT
  .bss    : { *(.bss) *(.bss.*) *(COMMON) } > BOOT
  /DISCARD/ : { *(.comment) *(.note*) }
}
