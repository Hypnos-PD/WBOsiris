#!/usr/bin/env bash
set -euo pipefail

card_root=cards
min_playable=0
min_percent=0
list_unplayable=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --min-playable) min_playable=${2:?missing value for --min-playable}; shift 2 ;;
    --min-percent) min_percent=${2:?missing value for --min-percent}; shift 2 ;;
    --list-unplayable) list_unplayable=1; shift ;;
    *) card_root=$1; shift ;;
  esac
done
total=$(find "$card_root" -name '*.wbo' -type f | wc -l | tr -d ' ')
unplayable=$(rg -l '^\s*unplayable;\s*$' "$card_root" --glob '*.wbo' | wc -l | tr -d ' ' || true)
playable=$((total - unplayable))
ratio=$((playable * 100 / total))
printf 'cards=%s\nplayable=%s\nunplayable=%s\nplayable_percent=%s\n' "$total" "$playable" "$unplayable" "$ratio"

if (( playable < min_playable )); then
  printf 'error: playable card count %s is below minimum %s\n' "$playable" "$min_playable" >&2
  exit 1
fi
if (( ratio < min_percent )); then
  printf 'error: playable percentage %s is below minimum %s\n' "$ratio" "$min_percent" >&2
  exit 1
fi
if (( list_unplayable )); then
  printf 'unplayable_sources:\n'
  rg -l '^\s*unplayable;\s*$' "$card_root" --glob '*.wbo' | sort || true
fi

if [[ "$total" -eq 0 ]]; then
  printf 'error: no card sources found\n' >&2
  exit 1
fi
