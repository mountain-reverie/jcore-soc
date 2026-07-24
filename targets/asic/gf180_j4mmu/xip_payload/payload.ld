OUTPUT_FORMAT("elf32-sh")
OUTPUT_ARCH(sh)
ENTRY(_start)
MEMORY {
  FLASH (rx) : ORIGIN = 0x14000000, LENGTH = 4K
}
SECTIONS {
  .text 0x14000000 : { *(.text) *(.text.*) } > FLASH
  /DISCARD/ : { *(.comment) *(.note*) }
}
