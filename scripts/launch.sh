#!/bin/sh
# Launch the local workbench.
#
# Three situations are handled:
#   * the built bundle (dist/)  -> plain python3, no uv needed, zero install
#   * the source tree           -> uv run, so the managed environment is used
#   * source tree without uv    -> plain python3 on the bridge module
set -e
cd -- "$(dirname -- "$0")"

if [ -f bridge.py ]; then
  exec python3 bridge.py "$@"
fi

if command -v uv >/dev/null 2>&1 && [ -f ../pyproject.toml ]; then
  cd ..
  exec uv run shiguang-study "$@"
fi

if [ -f ../src/shiguang_study/bridge.py ]; then
  exec python3 ../src/shiguang_study/bridge.py "$@"
fi

echo "Neither uv nor the bridge module was found."
echo "Build the bundle first:  python3 scripts/build.py"
printf 'Press Enter to close...'
read -r _ || true
exit 1
