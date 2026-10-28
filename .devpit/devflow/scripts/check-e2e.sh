#!/bin/sh
# Devflow check `e2e`: Playwright visual tests at phone and desktop sizes.
# It runs from the project's root, wherever it is started; a report
# written to $DEVPIT_REPORT_DIR is read as the check's evidence, and the
# screenshots of each test land in $DEVPIT_REPORT_DIR/screenshots.
set -e
cd "$(dirname "$0")/../../.."
DEVPIT_REPORT_DIR="${DEVPIT_REPORT_DIR:-.devpit/devflow/runs/local}"
export DEVPIT_REPORT_DIR
mkdir -p "$DEVPIT_REPORT_DIR"
# Absolute, since Playwright runs from web/.
report_dir="$(cd "$DEVPIT_REPORT_DIR" && pwd)"
E2E_JUNIT_FILE="$report_dir/playwright.xml"
E2E_OUTPUT_DIR="$report_dir/screenshots"
export E2E_JUNIT_FILE E2E_OUTPUT_DIR
make e2e
