#!/usr/bin/env bash
# Run at the design step, before changing implementation files.
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
exec go run ./tools/architecture -design "${1:-.architecture/design.json}"
