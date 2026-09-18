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
placeholder_report=$(node - "$card_root" <<'NODE'
// 骨架占位符是"effect 块里只有一条 unplayable;"；真正无法使用的卡（例如未来核心）
// 还带有融合块，因此不会被算成未实现。输出第一行是数量，其余行是文件路径。
const {readdirSync, readFileSync} = require('fs');
const {join} = require('path');

function walk(dir, out = []) {
  for (const entry of readdirSync(dir, {withFileTypes: true})) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) walk(path, out);
    else if (entry.isFile() && entry.name.endsWith('.wbo')) out.push(path);
  }
  return out;
}

function isPlaceholder(text) {
  const open = text.indexOf('effect {');
  if (open < 0) return false;
  let depth = 0;
  let end = -1;
  for (let i = text.indexOf('{', open); i < text.length; i++) {
    if (text[i] === '{') depth++;
    else if (text[i] === '}') {
      depth--;
      if (depth === 0) {
        end = i;
        break;
      }
    }
  }
  if (end < 0) return false;
  const body = text.slice(text.indexOf('{', open) + 1, end)
    .replace(/<<[^\n]*/g, '')
    .replace(/\s+/g, '');
  return body === 'unplayable;';
}

const list = walk(process.argv[2]).filter(path => isPlaceholder(readFileSync(path, 'utf8'))).sort();
process.stdout.write(list.length + '\n' + list.join('\n') + (list.length ? '\n' : ''));
NODE
)
unplayable=$(printf '%s\n' "$placeholder_report" | head -1)
unplayable_list=$(printf '%s\n' "$placeholder_report" | tail -n +2)
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
  printf '%s\n' "$unplayable_list" | sed '/^$/d'
fi

if [[ "$total" -eq 0 ]]; then
  printf 'error: no card sources found\n' >&2
  exit 1
fi
