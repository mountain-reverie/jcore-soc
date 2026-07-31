#!/usr/bin/env python3
"""Render the flat pad_ring routed DEF to a PNG (die outline, pads, soc child
macros, routed metal). Used by the CI gf180-die-area job to attach a visual.

Usage: render_def.py <routed.def> <config.json> <out.png>
"""
import re, os, sys, json
import matplotlib; matplotlib.use('Agg')
import matplotlib.pyplot as plt
from matplotlib.patches import Rectangle
import matplotlib.collections as mc
from matplotlib.lines import Line2D

DEF, CFG, OUT = sys.argv[1], sys.argv[2], sys.argv[3]
U = 2000.0  # GF180 DEF DBU per micron
HERE = os.path.dirname(os.path.abspath(CFG))
txt = open(DEF).read()
m = re.search(r'DIEAREA \( (\d+) (\d+) \) \( (\d+) (\d+) \)', txt)
DW, DH = int(m.group(3)) / U, int(m.group(4)) / U

cfg = json.load(open(CFG))
SZ = {}
for mk, mv in cfg['MACROS'].items():
    lp = mv['lef'][0]
    lp = lp.replace('dir::', HERE + '/').replace('pdk_dir::', os.environ.get('PDK_ROOT', '') + '/')
    try:
        s = re.search(r'SIZE ([\d.]+) BY ([\d.]+)', open(lp).read())
        SZ[mk] = (float(s.group(1)), float(s.group(2)))
    except Exception:
        SZ[mk] = (60, 60)

# classify: soc child macros vs IO pads, for colour
def col(cell):
    if cell.startswith('gf180mcu_fd_io__dvdd') or cell.startswith('gf180mcu_fd_io__dvss'):
        return '#b33'          # power pads
    if cell.startswith('gf180mcu_fd_io__'):
        return '#38c'          # signal pads
    return '#6a6'              # soc child macro

comp = re.search(r'COMPONENTS \d+ ;(.*?)END COMPONENTS', txt, re.S).group(1)
insts = []
for mm in re.finditer(r'-\s+\S+\s+(\S+)\s+\+\s+(?:FIXED|PLACED)\s+\(\s*(-?\d+)\s+(-?\d+)', comp):
    if mm.group(1) in SZ:
        insts.append((mm.group(1), int(mm.group(2)) / U, int(mm.group(3)) / U))

# routed metal
LAYC = {'Metal1': '#999', 'Metal2': '#3a6', 'Metal3': '#c73', 'Metal4': '#39c', 'Metal5': '#93c'}
segs = {}
nm = re.search(r'\nNETS \d+ ;(.*?)\nEND NETS', txt, re.S)
if nm:
    layer = lx = ly = None
    for t in re.finditer(r'(?:(?:NEW|ROUTED)\s+(\w+))|\(\s*(\d+|\*)\s+(\d+|\*)\s*\)|(;)', nm.group(1)):
        if t.group(1):
            layer = t.group(1); lx = ly = None
        elif t.group(4):
            layer = lx = ly = None
        else:
            sx, sy = t.group(2), t.group(3)
            if sx is None:
                continue
            x = lx if sx == '*' else int(sx) / U
            y = ly if sy == '*' else int(sy) / U
            if layer and lx is not None and (x != lx or y != ly):
                segs.setdefault(layer, []).append(((lx, ly), (x, y)))
            lx, ly = x, y
total = sum(len(v) for v in segs.values())

fig, ax = plt.subplots(figsize=(10, 10 * DH / DW))
for lay, ss in segs.items():
    ax.add_collection(mc.LineCollection(ss, colors=LAYC.get(lay, '#ccc'), linewidths=0.1, alpha=0.5))
for cell, x, y in insts:
    w, h = SZ[cell]
    is_pad = cell.startswith('gf180mcu_fd_io__')
    ax.add_patch(Rectangle((x, y), w, h, fc=col(cell), ec='k', lw=0.6, alpha=0.85 if is_pad else 0.5))
ax.add_patch(Rectangle((0, 0), DW, DH, fill=False, ec='k', lw=2))
ax.set_xlim(-60, DW + 60); ax.set_ylim(-60, DH + 60); ax.set_aspect('equal')
ax.set_xlabel('µm'); ax.set_ylabel('µm')
ax.set_title(f'gf180_j4mmu — flat SoC + IO pad ring\n'
             f'die {DW:.0f}×{DH:.0f} = {DW * DH / 1e6:.2f} mm² · {total} routed segs '
             f'(KianV ref 20.1 mm²)', fontsize=11)
leg = [Line2D([0], [0], color='#38c', lw=6, label='signal pads'),
       Line2D([0], [0], color='#b33', lw=6, label='power pads'),
       Line2D([0], [0], color='#6a6', lw=6, label='soc child macros')]
ax.legend(handles=leg, fontsize=8, loc='upper right')
plt.tight_layout(); plt.savefig(OUT, dpi=150); plt.close()
print(f'render_def.py: wrote {OUT} — die {DW:.0f}×{DH:.0f}={DW*DH/1e6:.2f}mm², {len(insts)} placed, {total} segs')
