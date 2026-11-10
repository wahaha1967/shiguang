#!/usr/bin/env python3
"""Assemble the shipping bundle in dist/ from the source tree.

Source of truth:
    src/app/index.html         the single-file app
    src/shiguang_study/        the local AI bridge package
    scripts/launch.sh          POSIX launcher        -> dist/launch.command
    scripts/launch.bat         Windows launcher     -> dist/launch.bat
    licenses/                  third-party licenses  -> dist/licenses/

The bundle stays dependency-free: dist/bridge.py is a plain copy of the bridge
module and runs on any Python 3.9+ without uv or a virtual environment. uv only
manages the development environment in the source tree.

Usage:
    python3 scripts/build.py           # rebuild dist/
    python3 scripts/build.py --check   # verify dist/ matches the sources
"""
import shutil
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DIST = ROOT / 'dist'

# (source, name inside dist/, executable mode or None)
FILES = (
    (ROOT / 'src' / 'app' / 'index.html', 'study-room.html', None),
    (ROOT / 'src' / 'shiguang_study' / 'bridge.py', 'bridge.py', None),
    (ROOT / 'scripts' / 'launch.sh', 'launch.command', 0o755),
    (ROOT / 'scripts' / 'launch.bat', 'launch.bat', None),
)

BUNDLE_README = """Shiguang Study Room - offline workbench bundle
=================================================

This folder is the deliverable. Keep every file here together.

Start it
--------
  macOS    double-click  launch.command
  Windows  double-click  launch.bat
  any OS   python3 bridge.py

The server opens http://127.0.0.1:4312/ in your browser. Keep the terminal
window open; press Ctrl+C to stop it.

Offline use
-----------
  study-room.html can also be opened directly by double-clicking it. All study
  features (wrong-answer book, spaced review, random practice, focus timer,
  statistics) work offline. Only photo/PDF recognition and "variation" practice
  need the server and a model API key.

AI keys
-------
  Set DASHSCOPE_API_KEY or DEEPSEEK_API_KEY in the environment before starting
  the server, or paste the key into the app's AI settings. Keys stay in memory
  only: they are never written to the HTML file or to exported backups.

Data note
---------
  data stored when opening study-room.html directly (file://) and data stored
  through the server (http://127.0.0.1:4312) live in different browser
  storage. Export a JSON backup before switching, then import it in
  "Preferences & Data".

licenses/
---------
  PDF.js (Mozilla, Apache-2.0) and the bundled fonts/decoders.
"""


def _stale(reason):
    print(f'  stale: {reason}')
    return 1


def check():
    """Compare dist/ against the sources. Returns the number of problems."""
    problems = 0
    for src, name, _mode in FILES:
        if not src.exists():
            problems += _stale(f'missing source {src.relative_to(ROOT)}')
            continue
        target = DIST / name
        if not target.exists():
            problems += _stale(f'missing dist/{name}')
        elif target.read_bytes() != src.read_bytes():
            problems += _stale(f'dist/{name} differs from {src.relative_to(ROOT)}')
    for src in sorted((ROOT / 'licenses').glob('*')):
        target = DIST / 'licenses' / src.name
        if not target.exists():
            problems += _stale(f'missing dist/licenses/{src.name}')
        elif target.read_bytes() != src.read_bytes():
            problems += _stale(f'dist/licenses/{src.name} differs from licenses/{src.name}')
    extra = [p for p in sorted(DIST.rglob('*')) if p.is_file()
             and p.name != 'README.txt'
             and p not in {DIST / name for _s, name, _m in FILES}
             and p.parent != DIST / 'licenses']
    for path in extra:
        problems += _stale(f'unexpected file dist/{path.relative_to(DIST)}')
    return problems


def build():
    if DIST.exists():
        shutil.rmtree(DIST)
    (DIST / 'licenses').mkdir(parents=True)
    for src, name, mode in FILES:
        if not src.exists():
            raise SystemExit(f'missing source: {src}')
        target = DIST / name
        shutil.copyfile(src, target)
        if mode is not None:
            target.chmod(mode)
    for src in sorted((ROOT / 'licenses').glob('*')):
        if src.is_file():
            shutil.copyfile(src, DIST / 'licenses' / src.name)
    (DIST / 'README.txt').write_text(BUNDLE_README, encoding='utf-8')


def main():
    if '--check' in sys.argv[1:]:
        problems = check()
        if problems:
            print(f'dist/ is out of date ({problems} problem(s)). Run: python3 scripts/build.py')
            return 1
        print('dist/ is up to date.')
        return 0
    build()
    print(f'Built {DIST.relative_to(ROOT)}/ ({len(FILES)} files + licenses + README.txt)')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
