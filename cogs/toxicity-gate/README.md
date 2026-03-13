# toxicity-gate

**CereBRO Phase 3 — Layer 0 (Brainstem Reflexes)**

PURE deterministic COG. Fast keyword/pattern blocklist screening with
word boundary checking.

## Algorithm

1. Load blocklist (one term per line)
2. For each turn, scan text against all blocklist terms
3. Word boundary matching: character before and after match must be
   non-alphanumeric (prevents "assassinate" matching "ass")
4. Case-insensitive by default

## Config

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| blocklist_path | string | data/blocklists/default.txt | Path to blocklist file |
| case_sensitive | bool | false | Case-sensitive matching |
