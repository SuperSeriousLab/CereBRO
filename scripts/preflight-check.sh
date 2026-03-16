#!/bin/bash
# Verify all services before starting nightly loop
set -euo pipefail

SERVICES=(
    "Ollama|http://10.70.70.14:11434/api/tags"
    "SLR|http://192.168.14.69:8080/health"
    "Sophrim|http://192.168.14.65:8090/health"
)

FAILED=0
for svc in "${SERVICES[@]}"; do
    NAME="${svc%%|*}"
    URL="${svc##*|}"
    if curl -sf --max-time 5 "$URL" > /dev/null 2>&1; then
        echo "OK: $NAME"
    else
        echo "FAIL: $NAME at $URL" >&2
        ((FAILED++))
    fi
done

[[ "$FAILED" -gt 0 ]] && { echo "PREFLIGHT: $FAILED services down." >&2; exit 1; }
echo "PREFLIGHT: all healthy"
