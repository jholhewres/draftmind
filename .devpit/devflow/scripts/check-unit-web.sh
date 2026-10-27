#!/bin/sh
# Devflow check `unit-web`.
# It runs from the project's root, wherever it is started; a report
# written to $DEVPIT_REPORT_DIR is read as the check's evidence.
set -e
cd "$(dirname "$0")/../../.."
DEVPIT_REPORT_DIR="${DEVPIT_REPORT_DIR:-.devpit/devflow/runs/local}"
export DEVPIT_REPORT_DIR
mkdir -p "$DEVPIT_REPORT_DIR"
pnpm -C web exec vitest run --reporter=default --reporter=junit --outputFile.junit="$DEVPIT_REPORT_DIR/vitest.xml"
