#!/usr/bin/env node
// 条件范围候选筛查工具。
//
// 用途：找出"条件句在中文翻译里被句号拆开、但英文把后续效果并进条件里"的卡牌，
// 以及"条件依赖所选卡牌身份"的卡牌。这类卡牌最容易把效果写在错误的范围内
// （90064320『天书深渊』就是这么翻车的）。
//
// 用法：
//   node scripts/card_scope_screen.mjs                     # 用默认路径跑全部候选
//   node scripts/card_scope_screen.mjs --cards <cards.json> --rules <cards 目录>
//
// 输出三个清单：
//   A 句数错位：中文/日文句子比英文多，且中文含条件词。
//   B 所选卡身份条件：日文出现「〜を選んだなら」，且条件之后还有别的句子。
//   C 规则侧 require：WBO 的非法术卡里仍在用 require 的位置（应为 choose）。

import fs from 'node:fs';
import path from 'node:path';

const CONDITION_WORDS = {
  chs: ['若', '如果', '倘若', '則', '则'],
  cht: ['若', '如果', '倘若', '則'],
  jpn: ['なら', '場合'],
  kor: ['경우', '라면'],
  eng: [' if ', 'if ', 'If ', ' unless '],
};

const SELECTED_CONDITION_PATTERNS = [/選んだなら/, /選んだ場合/, /選択したなら/, /選んでいたなら/];

export function stripText(text) {
  return String(text ?? '')
    .replace(/<hr\s*\/?>/gi, ' § ')
    .replace(/<[^>]*>/g, '')
    .replace(/\s+/g, ' ')
    .trim();
}

export function splitSentences(text, lang) {
  const cleaned = stripText(text);
  if (!cleaned) return [];
  const parts = lang === 'eng' ? cleaned.split(/(?<=[.!?])\s+/) : cleaned.split(/(?<=[。！？])\s*/);
  return parts.map(part => part.trim()).filter(Boolean);
}

export function hasCondition(sentences, lang) {
  const words = CONDITION_WORDS[lang] ?? [];
  return sentences.some(sentence => words.some(word => sentence.includes(word)));
}

export function cardTextLangs(card) {
  const skills = card.skill_texts ?? [];
  const pick = lang => stripText(skills.map(skill => skill[`text_${lang}`]).filter(Boolean).join(' § '));
  return { chs: pick('chs'), cht: pick('cht'), eng: pick('eng'), jpn: pick('jpn'), kor: pick('kor') };
}

// A 类：中/日文句数多于英文且中文含条件词——条件可能被句号切断了范围。
export function screenSentenceSplit(cards) {
  const rows = [];
  for (const card of cards) {
    const texts = cardTextLangs(card);
    if (!texts.chs || !texts.eng) continue;
    const chs = splitSentences(texts.chs, 'chs');
    const eng = splitSentences(texts.eng, 'eng');
    const jpn = splitSentences(texts.jpn, 'jpn');
    if (!hasCondition(chs, 'chs')) continue;
    if (chs.length > eng.length || chs.length > jpn.length) {
      rows.push({
        id: card.card_id,
        name: card.name_chs,
        sentences: { chs: chs.length, eng: eng.length, jpn: jpn.length },
        chs,
        eng,
      });
    }
  }
  return rows;
}

// B 类：日文用「〜を選んだなら」，且条件句之后还有句子——需要核对条件范围。
export function screenSelectedCondition(cards) {
  const rows = [];
  for (const card of cards) {
    const texts = cardTextLangs(card);
    if (!SELECTED_CONDITION_PATTERNS.some(pattern => pattern.test(texts.jpn))) continue;
    rows.push({
      id: card.card_id,
      name: card.name_chs,
      jpn: splitSentences(texts.jpn, 'jpn'),
      chs: splitSentences(texts.chs, 'chs'),
      eng: splitSentences(texts.eng, 'eng'),
    });
  }
  return rows;
}

// C 类：WBO 规则里非法术卡仍在用 require 的位置。
// 官方 QA：只有"打出时需要选卡牌"的法术才会因为选不到目标而不能打出；
// 随从与护符没有可选对象时照样能打出、能力照常结算，因此应当写 choose。
export function scanRuleRequires(ruleFiles) {
  const rows = [];
  for (const file of ruleFiles) {
    const source = fs.readFileSync(file, 'utf8');
    const type = (source.match(/^\s*type\s+(\w+);/m) ?? [])[1];
    if (!type || type === 'spell') continue;
    const lines = source.split('\n');
    const stack = [];
    for (let index = 0; index < lines.length; index += 1) {
      const line = lines[index];
      const opens = (line.match(/\{/g) ?? []).length;
      const closes = (line.match(/\}/g) ?? []).length;
      const block = (line.match(/^\s*([a-z_]+)\b[^;{]*\{\s*$/) ?? [])[1];
      if (opens > closes && block) stack.push(block);
      if (/^\s*require\b/.test(line)) {
        rows.push({
          id: path.basename(file, '.wbo'),
          type,
          block: stack[stack.length - 1] ?? 'root',
          file,
          line: index + 1,
        });
      }
      if (closes > opens) {
        for (let pop = 0; pop < closes - opens; pop += 1) stack.pop();
      }
    }
  }
  return rows;
}

export function collectRuleFiles(dir) {
  const out = [];
  const walk = current => {
    for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
      const full = path.join(current, entry.name);
      if (entry.isDirectory()) walk(full);
      else if (full.endsWith('.wbo')) out.push(full);
    }
  };
  walk(dir);
  return out.sort();
}

export function formatReport({ sentenceSplit, selectedCondition, ruleRequires }) {
  const lines = [];
  lines.push(`A 句数错位候选：${sentenceSplit.length}`);
  for (const row of sentenceSplit) {
    lines.push(
      `  ${row.id} ${row.name}  chs${row.sentences.chs}/eng${row.sentences.eng}/jpn${row.sentences.jpn}`,
    );
    lines.push(`     CHS: ${row.chs.join(' | ')}`);
    lines.push(`     ENG: ${row.eng.join(' | ')}`);
  }
  lines.push('');
  lines.push(`B 所选卡身份条件候选：${selectedCondition.length}`);
  for (const row of selectedCondition) {
    lines.push(`  ${row.id} ${row.name}`);
    lines.push(`     JPN: ${row.jpn.join(' | ')}`);
    lines.push(`     ENG: ${row.eng.join(' | ')}`);
  }
  lines.push('');
  lines.push(`C 非法术卡仍在用 require 的位置：${ruleRequires.length}`);
  for (const row of ruleRequires) {
    lines.push(`  ${row.id} (${row.type}) ${row.block}  ${row.file}:${row.line}`);
  }
  return lines.join('\n');
}

function main(argv) {
  const cardsPath = valueAfter(argv, '--cards') ?? '../WBArts/data/cards.json';
  const rulesDir = valueAfter(argv, '--rules') ?? 'cards';
  const cards = JSON.parse(fs.readFileSync(cardsPath, 'utf8'));
  const report = formatReport({
    sentenceSplit: screenSentenceSplit(cards),
    selectedCondition: screenSelectedCondition(cards),
    ruleRequires: scanRuleRequires(collectRuleFiles(rulesDir)),
  });
  console.log(report);
}

function valueAfter(argv, flag) {
  const index = argv.indexOf(flag);
  return index >= 0 ? argv[index + 1] : undefined;
}

if (process.argv[1] && import.meta.url === `file://${path.resolve(process.argv[1])}`) {
  main(process.argv.slice(2));
}
