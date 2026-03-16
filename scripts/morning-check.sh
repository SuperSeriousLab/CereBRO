#!/bin/bash
CEREBRO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
DATE=$(date +%Y-%m-%d)

echo "=== CereBRO Nightly — $DATE ==="
echo ""

# Alerts first
ALERT="$CEREBRO_DIR/data/generation/logs/LATEST_ALERT.md"
if [[ -f "$ALERT" ]]; then
    echo "*** WATCHDOG ALERT ***"
    echo ""
    cat "$ALERT"
    echo ""
    echo "---"
    echo ""
fi

# Morning report
REPORT="$CEREBRO_DIR/data/generation/logs/$DATE/morning-report.md"
if [[ -f "$REPORT" ]]; then
    cat "$REPORT"
else
    echo "No report for $DATE."
    echo ""
    echo "Troubleshoot:"
    echo "  crontab -l | grep cerebro"
    echo "  ls $CEREBRO_DIR/data/generation/logs/"
    echo "  tail -20 $CEREBRO_DIR/data/generation/logs/cron.log"
fi

echo ""
echo "---"
echo "Tip: alias morning='$CEREBRO_DIR/scripts/morning-check.sh'"
