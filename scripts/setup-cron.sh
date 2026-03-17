#!/bin/bash
set -euo pipefail

CEREBRO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
USER_HOME=$(eval echo ~$USER)

echo "This will install two cron entries (times in CET/CEST, system runs UTC):"
echo "  1. Nightly loop at 2:00 AM CET (1:00 UTC in winter, 0:00 UTC in summer)"
echo "  2. Watchdog at 5:00 AM CET (4:00 UTC in winter, 3:00 UTC in summer)"
echo ""
echo "CereBRO dir: $CEREBRO_DIR"
echo ""
read -p "Proceed? [y/N] " -n 1 -r
echo
[[ ! $REPLY =~ ^[Yy]$ ]] && { echo "Cancelled."; exit 0; }

# Remove existing cerebro cron entries
EXISTING=$(crontab -l 2>/dev/null | grep -v 'nightly-loop\|nightly-watchdog' || true)

# Add new entries
echo "$EXISTING
# CereBRO Nightly Lamarckian Loop — 2:00 AM CET = 1:00 UTC (winter) / 0:00 UTC (summer)
0 1 * * * $CEREBRO_DIR/scripts/nightly-loop.sh 50 >> $CEREBRO_DIR/data/generation/logs/cron.log 2>&1
# CereBRO Watchdog — 5:00 AM CET = 4:00 UTC (winter) / 3:00 UTC (summer)
0 4 * * * $CEREBRO_DIR/scripts/nightly-watchdog.sh >> $CEREBRO_DIR/data/generation/logs/watchdog.log 2>&1
" | crontab -

echo ""
echo "Cron installed:"
crontab -l | grep -i cerebro
echo ""
echo "To verify: crontab -l"
echo "To remove: crontab -l | grep -v cerebro | crontab -"
