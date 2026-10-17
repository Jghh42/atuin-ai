#!/bin/sh
# Packaging requires Python 3 stdlib and the exact go.mod toolchain.
set -eu
exec python3 -B -I "$(dirname "$0")/package.py" "$@"
