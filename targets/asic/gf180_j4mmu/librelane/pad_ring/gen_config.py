import json,re,sys,os
# paths resolve relative to this script (pad_ring/), so it runs from anywhere
BASE=os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
CHAN=float(sys.argv[1]) if len(sys.argv)>1 else 150.0
PADH=350.0; PW=75.0; OFF=CHAN+PADH; COR=380.0
tc=json.load(open(f'{BASE}/top/config.json'))
lefmap={'cpus':'cpus','icache_adapter':'icache_2k','dcache_adapter':'dcache_2k','devices':'soc_cluster.devices','qspi_flash_ctrl':'qspi_flash','sdram_ctrl':'smoke'}
lefname={'cpus':'cpus','icache_adapter':'icache_adapter','dcache_adapter':'dcache_adapter','devices':'devices','qspi_flash_ctrl':'qspi_flash_ctrl','sdram_ctrl':'sdram_ctrl'}
CW,CHt=tc['DIE_AREA'][2],tc['DIE_AREA'][3]
DW=round(CW+2*OFF); DH=round(CHt+2*OFF)
MACROS={}
for mk,mv in tc['MACROS'].items():
    d=f'../{lefmap[mk]}/runs/smoke/final'
    MACROS[mk]={'lef':[f'dir::{d}/lef/{lefname[mk]}.lef'],'gds':[f'dir::{d}/gds/{lefname[mk]}.gds'],'instances':{}}
    for inst,v in mv['instances'].items():
        x,y=v['location']
        MACROS[mk]['instances'][f'u_soc.{inst}']={'location':[round(x+OFF,2),round(y+OFF,2)],'orientation':v['orientation']}
txt=open(f'{BASE}/pad_ring/pad_ring.v').read()
sig=[m.group(1) for m in re.finditer(r'gf180mcu_fd_io__bi_t (pad_\w+)',txt)]
pw=[('dvdd',m.group(1)) for m in re.finditer(r'gf180mcu_fd_io__dvdd (pwr_\w+)',txt)]+[('dvss',m.group(1)) for m in re.finditer(r'gf180mcu_fd_io__dvss (pwr_\w+)',txt)]
# interleave power evenly among signal pads, then wrap all around the perimeter
allp=[('bi_t',p) for p in sig]
step=max(1,len(sig)//len(pw))
for k,(cell,inst) in enumerate(pw):
    allp.insert(min(len(allp),(k+1)*step+k),(cell,inst))
N=len(allp)
Ltb=DW-2*COR; Llr=DH-2*COR; per=2*(Ltb+Llr)
nb=round(N*Ltb/per); nr=round(N*Llr/per); nt=nb; nl=N-nb-nr-nt
IO='pdk_dir::libs.ref/gf180mcu_fd_io'; GDS=f'{IO}/gds/gf180mcu_fd_io.gds'
for cell in ('bi_t','dvdd','dvss'):
    MACROS[f'gf180mcu_fd_io__{cell}']={'lef':[f'{IO}/lef/gf180mcu_fd_io__{cell}.lef'],'gds':[GDS],'instances':{}}
cm={'bi_t':'gf180mcu_fd_io__bi_t','dvdd':'gf180mcu_fd_io__dvdd','dvss':'gf180mcu_fd_io__dvss'}
idx=0
for e,n in [('S',nb),('E',nr),('N',nt),('W',nl)]:
    for j in range(n):
        cell,inst=allp[idx]; idx+=1
        if e in 'SN':
            x=COR+(DW-2*COR-PW)*j/max(n-1,1); y=0.0 if e=='S' else DH-PADH; o='N' if e=='S' else 'S'
        else:
            y=COR+(DH-2*COR-PW)*j/max(n-1,1); x=0.0 if e=='W' else DW-PADH; o='E' if e=='W' else 'W'
        MACROS[cm[cell]]['instances'][inst]={'location':[round(x,2),round(y,2)],'orientation':o}
cfg={'DESIGN_NAME':'pad_ring','VERILOG_FILES':['dir::pad_ring.v','dir::pad_cells_bb.v','dir::../top/soc.v','dir::../top/soc_macros_bb.v'],
 'CLOCK_PORT':'clk_sys_PAD','CLOCK_PERIOD':40.0,'FP_SIZING':'absolute','DIE_AREA':[0,0,DW,DH],
 'CORE_AREA':[PADH,PADH,DW-PADH,DH-PADH],'PL_TARGET_DENSITY_PCT':12,
 'FP_PDN_VPITCH':418.0,'FP_PDN_HPITCH':418.0,'FP_PDN_VWIDTH':3.1,'FP_PDN_HWIDTH':3.1,'FP_PDN_VOFFSET':50.0,'FP_PDN_HOFFSET':50.0,
 'RUN_LINTER':False,'RUN_MCSTA':False,'MACROS':MACROS,
 '_comment':f'FLAT pad ring, pads spread EVENLY around the perimeter (S{nb}/E{nr}/N{nt}/W{nl}). channel {CHAN:.0f}um.'}
json.dump(cfg,open(f'{BASE}/pad_ring/config.json','w'),indent=2)
print(f"FLAT even: die {DW}x{DH}={DW*DH/1e6:.1f}mm2 | pads S{nb}/E{nr}/N{nt}/W{nl}")
