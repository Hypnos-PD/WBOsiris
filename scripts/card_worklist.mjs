#!/usr/bin/env node
// 全卡覆盖率工作面：统计已写/未写卡牌、按卡包与文本长度分档，给出下一批的候选。
//
// 用法：
//   node scripts/card_worklist.mjs [--cards ../WBArts/data/cards.json] [--root .] [--pack 10002] [--limit 30]
import {readFileSync, readdirSync, statSync} from 'node:fs';
import {join, resolve, basename} from 'node:path';

const args = process.argv.slice(2);
const option = (name, fallback) => {
  const index = args.indexOf(`--${name}`);
  return index >= 0 && args[index + 1] ? args[index + 1] : fallback;
};
const root = resolve(option('root', '.'));
const source = resolve(root, option('cards', '../WBArts/data/cards.json'));
const pack = option('pack', '');
const limit = Number(option('limit', '0'));

function walk(dir, out = []) {
  for (const entry of readdirSync(dir, {withFileTypes: true})) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) walk(path, out);
    else if (entry.isFile() && entry.name.endsWith('.wbo')) out.push(path);
  }
  return out;
}

const files = walk(join(root, 'cards'));
const written = new Map();
const unfinished = new Set();
for (const path of files) {
  const id = basename(path, '.wbo');
  written.set(id, path);
  if (/^\s*unplayable;\s*$/m.test(readFileSync(path, 'utf8'))) unfinished.add(id);
}

const cards = JSON.parse(readFileSync(source, 'utf8'));
const plain = card => (card.skill_texts || [])
  .map(entry => String(entry.text_chs || ''))
  .join(' ')
  .replace(/<[^>]+>/g, '')
  .replace(/\s+/g, '');
const difficulty = card => {
  const length = plain(card).length;
  if (length === 0) return '白板';
  if (length <= 40) return '短(≤40字)';
  if (length <= 100) return '中(41–100字)';
  return '长(>100字)';
};

const missing = cards.filter(card => !written.has(String(card.card_id)));
const done = cards.filter(card => written.has(String(card.card_id)) && !unfinished.has(String(card.card_id)));
const pending = cards.filter(card => unfinished.has(String(card.card_id)));

const byPack = new Map();
for (const card of cards) {
  const key = String(card.card_set_id);
  const row = byPack.get(key) || {total: 0, done: 0, pending: 0, missing: 0};
  row.total += 1;
  const id = String(card.card_id);
  if (!written.has(id)) row.missing += 1;
  else if (unfinished.has(id)) row.pending += 1;
  else row.done += 1;
  byPack.set(key, row);
}

console.log(`卡表 ${source}`);
console.log(`总计 ${cards.length} · 已完成 ${done.length} · 未实现(骨架) ${pending.length} · 未导入 ${missing.length}`);
console.log('');
console.log('卡包      总数  已完成  未实现  未导入');
for (const [key, row] of [...byPack.entries()].sort((a, b) => Number(a[0]) - Number(b[0]))) {
  console.log(`${key.padEnd(8)} ${String(row.total).padStart(6)} ${String(row.done).padStart(7)} ${String(row.pending).padStart(7)} ${String(row.missing).padStart(7)}`);
}

const buckets = new Map();
for (const card of missing.concat(pending)) {
  const key = difficulty(card);
  buckets.set(key, (buckets.get(key) || 0) + 1);
}
console.log('');
console.log('未完成卡按文本复杂度：');
for (const [key, count] of [...buckets.entries()].sort((a, b) => b[1] - a[1])) {
  console.log(`  ${key}: ${count}`);
}

const scope = pack ? cards.filter(card => String(card.card_set_id) === pack) : cards;
const candidates = scope
  .filter(card => {
    const id = String(card.card_id);
    return !written.has(id) || unfinished.has(id);
  })
  .sort((a, b) => plain(a).length - plain(b).length);
if (candidates.length) {
  console.log('');
  console.log(`下一批候选${pack ? `（卡包 ${pack}）` : ''}，按文本由短到长：`);
  for (const card of candidates.slice(0, limit || 20)) {
    console.log(`  ${card.card_id} [${difficulty(card)}] ${card.name_chs} :: ${plain(card).slice(0, 48)}`);
  }
}

void statSync;
