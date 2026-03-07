#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

for dir in orpc prisma express react-router; do
  echo "==> $dir: installing dependencies"
  (cd "$SCRIPT_DIR/$dir" && npm install --silent)
  echo "==> $dir: building"
  (cd "$SCRIPT_DIR/$dir" && npm run build --silent)
done

echo "Done. All scanners built."
