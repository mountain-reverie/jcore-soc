# OpenRAM SPIKE config: single-port (1RW) SRAM matching the RAM_32x1x512
# geometry from lib/memory_tech_lib/ram_2rw.vhd (mem_layout_t.RAM_32x1x512):
# 512 words x 32 bits, one bank.
#
# NOTE (see ../../DECISION.md): GF180 bitcell support in this OpenRAM version
# (see README.md for exact commit/version) registers only tech_modules["bitcell_1port"]
# in technology/gf180mcu/tech/tech.py -- there is no tech_modules["bitcell_2port"]
# entry for gf180mcu (unlike sky130, which registers bitcell_2port/replica_bitcell_2port/
# dummy_bitcell_2port). This config therefore requests a single 1RW port; it is NOT
# possible to request num_rw_ports=2 or num_r_ports=1,num_w_ports=1 for gf180mcu with
# this OpenRAM tech tree as shipped. This macro stands in for one WRITE-only or
# READ-only "half" of the cache's logical 1RW+1W port pair (see DECISION.md fallback).

word_size = 32
num_words = 512
num_rw_ports = 1
num_r_ports = 0
num_w_ports = 0

tech_name = "gf180mcu"

process_corners = ["TT"]
supply_voltages = [3.3]
temperatures = [25]
nominal_corner_only = True

route_supplies = "ring"
check_lvsdrc = False
analytical_delay = True

output_path = "OUTPUT_PATH_PLACEHOLDER"
output_name = "sram_32x512_1rw_gf180"

num_spare_rows = 0
num_spare_cols = 0
