import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {test} from 'node:test';

import {
  collectRuleFiles,
  missingCards,
  referencedCardIDs,
} from './card_scenario_coverage.mjs';

test('collectRuleFiles 递归收集 wbotest 文件', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'wbo-scenario-'));
  try {
    fs.mkdirSync(path.join(dir, '10009'));
    fs.writeFileSync(path.join(dir, '10009', 'a.wbotest'), 'wbotest 0.1.0;');
    fs.writeFileSync(path.join(dir, 'note.txt'), 'x');
    assert.deepEqual(collectRuleFiles(dir).map(file => path.basename(file)), ['a.wbotest']);
  } finally {
    fs.rmSync(dir, {recursive: true, force: true});
  }
});

test('referencedCardIDs 读取场景里的八位卡牌 ID', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'wbo-scenario-'));
  try {
    const file = path.join(dir, 'a.wbotest');
    fs.writeFileSync(file, 'hand { follower source = 10101110; } deck top { spell other = 90071220; }');
    assert.deepEqual([...referencedCardIDs([file])].sort(), ['10101110', '90071220']);
  } finally {
    fs.rmSync(dir, {recursive: true, force: true});
  }
});

test('missingCards 列出没有被任何场景引用的卡', () => {
  const cards = [{card_id: 1}, {card_id: 2}, {card_id: 3}];
  assert.deepEqual(missingCards(cards, new Set(['1', '3'])), ['2']);
});
