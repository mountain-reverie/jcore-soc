OUTPUT_FORMAT("elf32-sh")
OUTPUT_ARCH(sh)
ENTRY(_start)
MEMORY { SPRAM (rwx) : ORIGIN = 0x10000000, LENGTH = 128K }
SECTIONS {
  . = 0x10000000;
  .text   : { KEEP(*(.text.start)) *(.text) *(.text.*) } > SPRAM
  .rodata : { *(.rodata) *(.rodata.*) } > SPRAM
  .data   : { *(.data) *(.data.*) } > SPRAM
  . = ALIGN(4); __bss_start = .;
  .bss (NOLOAD) : { *(.bss) *(.bss.*) *(COMMON) } > SPRAM
  . = ALIGN(4); __bss_end = .;
  . = ALIGN(8); __stack_top = . + 8K;
  .stack (NOLOAD) : { . += 8K; } > SPRAM
  /DISCARD/ : { *(.comment) *(.note*) }
}
