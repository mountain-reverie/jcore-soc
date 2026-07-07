/* xip.x -- linker script for the iCESugar XIP page-cache software test
   (xip_handler.s + xip_test.c). Layout, per the Task-6 brief's FINAL
   address map:

     - Resident set (vectors, crt0, _pf_handler, victim counter): the boot
       EBR at 0x00000000 (2 KiB, same physical block as icesugar.ld's BOOT
       region). NEVER paged -- the handler must not fault while handling a
       fault.
     - .text/.rodata (xip_test.c's code + the >4-page const table): VMA
       0x10800000 (the flash execute-in-place window), LMA 0x00100000 (the
       flash offset the XIP image is written at; the fill engine adds
       page<<12 internally so only the page number is ever passed to it).
       These bytes are NEVER directly loaded by the CPU -- they only become
       resident via the fill engine copying a page into a frame -- but the
       LMA is where the raw image bytes must be programmed into the
       behavioral/real flash.
     - .data/.bss/.stack (SPRAM, resident, demand-paging does not apply
       here): 0x10000000, 128 KiB.
*/
OUTPUT_FORMAT("elf32-sh")
OUTPUT_ARCH(sh)
ENTRY(_start)

MEMORY {
  BOOT   (rwx) : ORIGIN = 0x00000000, LENGTH = 2K
  SPRAM  (rwx) : ORIGIN = 0x10000000, LENGTH = 128K
  WINDOW (rx)  : ORIGIN = 0x10800000, LENGTH = 1M
}

FLASH_BASE = 0x00100000;

SECTIONS {
  .vectors 0x0 : { KEEP(*(.vectors)) } > BOOT

  /* Resident code/data: crt0 (_start), _pf_handler, _vbr_table, the
     round-robin victim counter. All from xip_handler.s. */
  .boot.text : {
    *(.boot.text) *(.boot.text.*)
  } > BOOT
  . = ALIGN(4);
  .boot.data : {
    *(.boot.data) *(.boot.data.*)
  } > BOOT

  /* Demand-paged execute-in-place window: xip_test.c's .text + .rodata
     (the >4-page const table). VMA in the window, LMA in flash. */
  .text 0x10800000 : AT(FLASH_BASE) {
    *(.text) *(.text.*)
    *(.rodata) *(.rodata.*)
  } > WINDOW

  /* Resident data: xip_test.c has no initialised globals, so .data is
     expected empty; keep it in SPRAM with VMA==LMA (no copy needed) in
     case that ever changes. */
  .data 0x10000000 : { *(.data) *(.data.*) } > SPRAM

  . = ALIGN(4); __bss_start = .;
  .bss (NOLOAD) : { *(.bss) *(.bss.*) *(COMMON) } > SPRAM
  . = ALIGN(4); __bss_end = .;

  . = ALIGN(8);
  .stack (NOLOAD) : { . += 4K; } > SPRAM
  __stack_top = .;

  /DISCARD/ : { *(.comment) *(.note*) }
}
