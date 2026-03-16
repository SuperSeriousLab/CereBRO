#!/bin/bash
set -euo pipefail

CEREBRO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
USER_HOME=$(eval echo ~$USER)

echo "This will install two cron entries:"
echo "  1. Nightly loop at 2:00 AM (50 conversations)"
echo "  2. Watchdog at 5:00 AM (3-hour buffer)"
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
# CereBRO Nightly Lamarckian Loop (installed $(date +%Y-%m-%d))
0 2 * * * $CEREBRO_DIR/scripts/nightly-loop.sh 50 >> $CEREBRO_DIR/data/generation/logs/cron.log 2>&1
# CereBRO Watchdog (3h after loop)
0 5 * * * $CEREBRO_DIR/scripts/nightly-watchdog.sh >> $CEREBRO_DIR/data/generation/logs/watchdog.log 2>&1
" | crontab -

echo ""
echo "Cron installed:"
crontab -l | grep -i cerebro
echo ""
echo "To verify: crontab -l"
echo "To remove: crontab -l | grep -v cerebro | crontab -"
