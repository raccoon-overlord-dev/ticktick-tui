#!/usr/bin/env python3
# Builds ttui's palette themes (internal/theme/palettes) from terminalcolors.com: the main
# variant of each theme on its dark and light tag pages, mapped from the theme's Ghostty file
# onto ttui's color roles. Run with `make palettes`; pages are cached in the second argument.
#
#   python3 scripts/palettes.py <out dir> [cache dir]
import html, json, re, sys, tempfile, urllib.request, pathlib

out = pathlib.Path(sys.argv[1])
cache = pathlib.Path(sys.argv[2] if len(sys.argv) > 2 else tempfile.mkdtemp(prefix='ttui-palettes-'))
out.mkdir(parents=True, exist_ok=True)
cache.mkdir(parents=True, exist_ok=True)

def get(url):
    f = cache / re.sub(r'[^a-z0-9]+', '_', url.lower())
    if not f.exists():
        req = urllib.request.Request(url, headers={'User-Agent': 'ttui-palette-gen'})
        f.write_bytes(urllib.request.urlopen(req, timeout=30).read())
    return f.read_text()

def unwrap(v):  # astro props: [0, value] or [1, [items]]
    if isinstance(v, list) and len(v) == 2 and v[0] in (0, 1):
        return unwrap(v[1])
    if isinstance(v, list):
        return [unwrap(x) for x in v]
    if isinstance(v, dict):
        return {k: unwrap(x) for k, x in v.items()}
    return v

def rgb(h): return [int(h[i:i+2], 16) for i in (1, 3, 5)]
def hexc(c): return '#%02x%02x%02x' % tuple(max(0, min(255, round(x))) for x in c)
def mix(a, b, t): return hexc([x + (y - x) * t for x, y in zip(rgb(a), rgb(b))])
def lum(h):
    c = [x / 255 for x in rgb(h)]
    c = [x / 12.92 if x <= 0.03928 else ((x + 0.055) / 1.055) ** 2.4 for x in c]
    return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2]
def contrast(a, b):
    la, lb = sorted([lum(a), lum(b)], reverse=True)
    return (la + 0.05) / (lb + 0.05)
def readable(c, bg, fg, need=3.0):  # pull c toward fg until it reads on bg
    t = 0
    while contrast(mix(c, fg, t), bg) < need and t < 1:
        t += 0.05
    return mix(c, fg, t)

made = []
for mode in ('dark', 'light'):
    page = html.unescape(get(f'https://terminalcolors.com/tags/{mode}/'))
    for m in re.finditer(r'props="(\{"theme".*?\})" ', page):
        th = unwrap(json.loads(m.group(1)))['theme']
        v = th['variants'][0]
        g = {}
        for line in get(f"https://terminalcolors.com/downloads/ghostty/{th['id']}-{v['id']}").splitlines():
            k, _, val = (x.strip() for x in line.partition('='))
            if k == 'palette':
                n, _, val = val.partition('=')
                g[int(n)] = val.strip()
            else:
                g[k] = val
        bg, fg = g['background'], g['foreground']
        sel = g.get('selection-background', mix(bg, fg, 0.15))
        if contrast(sel, bg) < 1.15 or contrast(sel, fg) < 3:
            sel = mix(bg, fg, 0.15)
        col = lambda i: readable(g[i], bg, fg)
        dim = readable(mix(bg, fg, 0.45), bg, fg, 2.3)
        roles = {
            'base': bg, 'surface': mix(bg, fg, 0.04), 'surface2': sel, 'line': mix(bg, fg, 0.2),
            'dim': dim, 'muted': readable(mix(bg, fg, 0.62), bg, fg, 3.2), 'sub': readable(mix(bg, fg, 0.8), bg, fg, 4.5), 'text': fg,
            'accent': readable(v['accent'], bg, fg), 'secondary': col(5), 'ok': col(2), 'info': col(4),
            'error': col(1), 'warn': col(3), 'p_high': col(1), 'p_med': col(3), 'p_low': col(4),
            'p_none': dim,
        }
        name = f"{th['id']}-{mode}"
        body = [f"# ticktick-tui theme: {th['name']} {v['name']} ({mode})",
                f"# Colors from terminalcolors.com/themes/{th['id']}/{v['id']}/, mapped onto ttui's roles",
                "# by scripts/palettes.py (make palettes): don't edit, regenerate.",
                f'name = "{th["id"]}"', f'family = "{th["name"]}"', f'variant = "{"" if v["name"] in ("Default", "Dark", "Light") else v["name"]}"',
                f'mode = "{mode}"', 'separator = "·"', '', '[colors]']
        body += [f'{k:<9} = "{c}"' for k, c in roles.items()]
        (out / f'{name}.toml').write_text('\n'.join(body) + '\n')
        made.append(name)
print(len(made), ' '.join(made))
