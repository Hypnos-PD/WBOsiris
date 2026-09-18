#!/usr/bin/env node
// 场景覆盖检查：全卡政策的第 2 条要求"每张卡都至少有一个测试场景"。
// 本脚本把卡表里的卡牌 ID 与 tests/**.wbotest 里出现的 ID 对照，列出漏掉的卡。
//
// 用法：
//   node scripts/card_scenario_coverage.mjs [--cards ../WBArts/data/cards.json] [--tests tests]
// 有漏掉的卡时以退出码 1 结束，方便当作门禁。

import fs from 'node:fs';
import path from 'node:path';

export function collectRuleFiles(dir) {
  const out = [];
  const walk = current => {
    for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
      const full = path.join(current, entry.name);
      if (entry.isDirectory()) walk(full);
      else if (full.endsWith('.wbotest')) out.push(full);
    }
  };
  walk(dir);
  return out.sort();
}

export function referencedCardIDs(files) {
  const ids = new Set();
  for (const file of files) {
    const source = fs.readFileSync(file, 'utf8');
    for (const match of source.matchAll(/\b\d{8}\b/g)) ids.add(match[0]);
  }
  return ids;
}

export function missingCards(cards, referenced) {
  return cards
    .map(card => String(card.card_id))
    .filter(id => !referenced.has(id))
    .sort();
}

function valueAfter(argv, flag) {
  const index = argv.indexOf(flag);
  return index >= 0 ? argv[index + 1] : undefined;
}

function main(argv) {
  const cardsPath = valueAfter(argv, '--cards') ?? '../WBArts/data/cards.json';
  const testsDir = valueAfter(argv, '--tests') ?? 'tests';
  const cards = JSON.parse(fs.readFileSync(cardsPath, 'utf8'));
  const referenced = referencedCardIDs(collectRuleFiles(testsDir));
  const missing = missingCards(cards, referenced);
  console.log(`卡表 ${cards.length} 张 · 场景引用 ${cards.length - missing.length} 张 · 未覆盖 ${missing.length} 张`);
  if (missing.length > 0) {
    console.log('未覆盖卡牌：' + missing.join(' '));
    process.exitCode = 1;
  }
}

if (process.argv[1] && import.meta.url === `file://${path.resolve(process.argv[1])}`) {
  main(process.argv.slice(2));
}
