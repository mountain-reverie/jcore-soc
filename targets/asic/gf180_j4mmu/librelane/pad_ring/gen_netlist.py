# Generate pad_ring.v : soc core + 58 bi_t signal pads + power pads, KianV-structured.
# bi_t pins: PAD(chip) A(core->pad out) Y(pad->core in) OE(out-en) IE(in-en)
#            ON CS SL PU PD PDRV0 PDRV1 (controls, tied) DVDD DVSS VDD VSS (power)
BIT='gf180mcu_fd_io__bi_t'; DVDD='gf180mcu_fd_io__dvdd'; DVSS='gf180mcu_fd_io__dvss'
# signal spec: (name, dir, core_out_net, core_in_net, oe_net)
#   dir: 'in'  -> Y=core_in, OE=0, IE=1
#        'out' -> A=core_out, OE=1, IE=0
#        'bi'  -> A=core_out, Y=core_in, OE=oe_net, IE=1
import os
DIR=os.path.dirname(os.path.abspath(__file__))
sig=[]
def add(nm,d,o=None,i=None,oe=None): sig.append((nm,d,o,i,oe))
# inputs
add('clk_sys','in',i='clk_sys'); add('reset','in',i='reset')
add('uart0_rx','in',i='uart0_rx'); add('spi2_miso','in',i='spi2_miso')
# outputs (scalar)
add('uart0_tx','out',o='uart0_tx'); add('spi2_clk','out',o='spi2_clk'); add('spi2_mosi','out',o='spi2_mosi')
add('qfl_cs_n','out',o='qfl_cs_n'); add('qfl_sck','out',o='qfl_sck')
add('sd_cmd_cas_n','out',o='sd_cmd_cas_n'); add('sd_cmd_cke','out',o='sd_cmd_cke')
add('sd_cmd_cs_n','out',o='sd_cmd_cs_n'); add('sd_cmd_ras_n','out',o='sd_cmd_ras_n'); add('sd_cmd_we_n','out',o='sd_cmd_we_n')
# output buses
for k in range(2): add(f'spi2_cs_{k}','out',o=f'spi2_cs[{k}]')
for k in range(13): add(f'sd_cmd_a_{k}','out',o=f'sd_cmd_a[{k}]')
for k in range(2): add(f'sd_cmd_ba_{k}','out',o=f'sd_cmd_ba[{k}]')
for k in range(2): add(f'sd_cmd_dqm_{k}','out',o=f'sd_cmd_dqm[{k}]')
# bidir
for k in range(4): add(f'qfl_io_{k}','bi',o=f'qfl_io_o[{k}]',i=f'qfl_io_i[{k}]',oe=f'qfl_io_oe[{k}]')
for k in range(16): add(f'sd_dq_{k}','bi',o=f'sd_dq_o[{k}]',i=f'sd_dq_i[{k}]',oe='sd_dq_oe')
for k in range(5): add(f'gpio_{k}','bi',o=f'gpio_do[{k}]',i=f'gpio_di[{k}]',oe="1'b1")
print(f"// {len(sig)} signal pads")
NPWR_DVDD=8; NPWR_DVSS=8

# Read the ACTUAL pin list of each IO cell from its LEF, so both the
# instantiations and the blackbox stubs connect only ports the PDK cell really
# has -- the gf180 cells are asymmetric (dvdd has DVDD/DVSS/VSS but no VDD;
# dvss has DVDD/DVSS/VDD but no VSS; bi_t has no ON) and hard-wiring a fixed
# port set makes LibreLane's yosys quit ("cell does not have a port named ...").
import glob
IOLEF=os.environ.get('IO_LEF_DIR','')
if not os.path.isdir(IOLEF):
    pats=[]
    if os.environ.get('PDK_ROOT'):
        pats.append(os.path.join(os.environ['PDK_ROOT'],'libs.ref/gf180mcu_fd_io/lef'))
    pats.append(os.path.expanduser('~/.ciel/ciel/gf180mcu/versions/*/gf180mcu*/libs.ref/gf180mcu_fd_io/lef'))
    for pat in pats:
        c=glob.glob(pat)
        if c: IOLEF=c[0]; break
if not os.path.isdir(IOLEF):
    sys.exit(f"gen_netlist: could not locate gf180mcu_fd_io LEF dir (set PDK_ROOT or IO_LEF_DIR); tried {pats}")
cellpins={c:[l.split()[1] for l in open(f'{IOLEF}/gf180mcu_fd_io__{c}.lef') if l.strip().startswith('PIN ')]
          for c in ('bi_t','dvdd','dvss')}

L=[]
L.append('/* pad_ring: soc core + GF180 IO pad ring (KianV-structured, 58 signals). Generated. */')
L.append('module pad_ring (')
padpins=[f'  inout {s[0]}_PAD' for s in sig]
padpins+=[f'  inout DVDD_{k}' for k in range(NPWR_DVDD)]+[f'  inout DVSS_{k}' for k in range(NPWR_DVSS)]
L.append(',\n'.join(padpins))
L.append(');')
# core power nets
L.append('  wire VDD, VSS, DVDD, DVSS;')
# soc-side nets (declare all soc ports as wires)
soc_ports={'clk_sys':1,'gpio_di':32,'gpio_do':32,'qfl_cs_n':1,'qfl_io_i':4,'qfl_io_o':4,'qfl_io_oe':4,'qfl_sck':1,
 'reset':1,'sd_cmd_a':13,'sd_cmd_ba':2,'sd_cmd_cas_n':1,'sd_cmd_cke':1,'sd_cmd_cs_n':1,'sd_cmd_dqm':2,'sd_cmd_ras_n':1,
 'sd_cmd_we_n':1,'sd_dq_i':16,'sd_dq_o':16,'sd_dq_oe':1,'spi2_clk':1,'spi2_cs':2,'spi2_miso':1,'spi2_mosi':1,'uart0_rx':1,'uart0_tx':1}
for p,w in soc_ports.items():
    L.append(f'  wire {"["+str(w-1)+":0] " if w>1 else ""}{p};')
# tie unbonded gpio_di[31:5] low
L.append("  assign gpio_di[31:5] = 27'b0;")
# instantiate signal pads -- connect only pins the bi_t cell actually has
def inst(cell, name, conn):
    ports=', '.join(f'.{p}({conn[p]})' for p in cellpins[cell] if p in conn)
    return f'  gf180mcu_fd_io__{cell} {name} ({ports});'
for nm,d,o,i,oe in sig:
    A = o if o else "1'b0"
    OE = "1'b1" if d=='out' else ("1'b0" if d=='in' else oe)
    IE = "1'b0" if d=='out' else "1'b1"
    conn={'PAD':f'{nm}_PAD','A':A,'OE':OE,'IE':IE,
          'CS':'VDD','SL':'VSS','PU':'VSS','PD':'VSS','PDRV0':'VDD','PDRV1':'VDD','ON':'VDD',
          'DVDD':'DVDD','DVSS':'DVSS','VDD':'VDD','VSS':'VSS'}
    if i: conn['Y']=i   # pad->core input only where used
    L.append(inst('bi_t',f'pad_{nm}',conn))
# power pads -- each cell connects only its own real pins to same-named nets
pw={'DVDD':'DVDD','DVSS':'DVSS','VDD':'VDD','VSS':'VSS'}
for k in range(NPWR_DVDD): L.append(inst('dvdd',f'pwr_dvdd_{k}',pw))
for k in range(NPWR_DVSS): L.append(inst('dvss',f'pwr_dvss_{k}',pw))
# soc core
conns=', '.join(f'.{p}({p})' for p in soc_ports)
L.append(f'  soc u_soc ({conns});')
L.append('endmodule')
open(os.path.join(DIR,'pad_ring.v'),'w').write('\n'.join(L)+'\n')
print(f"wrote pad_ring.v : {len(sig)} signal pads + {NPWR_DVDD+NPWR_DVSS} power pads")

# also emit the IO-cell blackbox stubs (pad_cells_bb.v) from the same pin lists
out=['/* GF180 IO pad cell blackbox stubs (generated) */']
for cell,pins in cellpins.items():
    out+=['(* blackbox *)',f'module gf180mcu_fd_io__{cell} ({", ".join(pins)});']+[f'  inout {p};' for p in pins]+['endmodule','']
open(os.path.join(DIR,'pad_cells_bb.v'),'w').write('\n'.join(out))
print("wrote pad_cells_bb.v")
