# Sourced by scripts/toolchain.sh and scripts/package.sh after they set ROOT.
# The toolchain locations under .tools/. Plain assignments only (no eval and no
# printed shell code), so any characters in ROOT stay plain text.
TOOLS="$ROOT/.tools"
UV="$TOOLS/uv-venv/bin/uv"
UV_CACHE_DIR="$TOOLS/uv-cache"
UV_PYTHON_INSTALL_DIR="$TOOLS/python"
UV_PYTHON_PREFERENCE=only-managed
UV_PROJECT_ENVIRONMENT="$TOOLS/research-venv"
UV_NO_PROGRESS=1
export UV_CACHE_DIR UV_PYTHON_INSTALL_DIR UV_PYTHON_PREFERENCE UV_PROJECT_ENVIRONMENT UV_NO_PROGRESS
