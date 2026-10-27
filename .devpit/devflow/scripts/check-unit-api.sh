#!/bin/sh
# Devflow check `unit-api`.
# It runs from the project's root, wherever it is started; a report
# written to $DEVPIT_REPORT_DIR is read as the check's evidence.
set -e
cd "$(dirname "$0")/../../.."
DEVPIT_REPORT_DIR="${DEVPIT_REPORT_DIR:-.devpit/devflow/runs/local}"
export DEVPIT_REPORT_DIR
mkdir -p "$DEVPIT_REPORT_DIR"
go -C api test -json ./... > "$DEVPIT_REPORT_DIR/go-test.json"
