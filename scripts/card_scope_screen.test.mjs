import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {test} from 'node:test';

import {
  scanRuleRequires,
  screenSelectedCondition,
  screenSentenceSplit,
  splitSentences,
  stripText,
} from './card_scope_screen.mjs';

const card = ({chs, eng, jpn, id = 90000000, name = '样例'}) => ({
  card_id: id,
  name_chs: name,
  skill_texts: [{text_chs: chs, text_eng: eng, text_jpn: jpn}],
});

test('stripText 把 <hr> 当成块分隔符并去掉标签', () => {
  assert.equal(stripText('甲。<hr>乙。<color=Keyword>丙</color>。'), '甲。 § 乙。丙。');
});

test('splitSentences 按语言切句', () => {
  assert.deepEqual(splitSentences('甲。乙。', 'chs'), ['甲。', '乙。']);
  assert.deepEqual(splitSentences('First. Second.', 'eng'), ['First.', 'Second.']);
});

test('中文句数多于英文且含条件词时进入 A 类候选', () => {
  const rows = screenSentenceSplit([
    card({
      chs: '选择战场上的1张卡牌，破坏该卡牌。若选择了自己的护符，则对对手的主战者造成2点伤害。将1张『天书深渊』加入手牌。',
      eng: 'Select a card on the field and destroy it. If you selected an allied amulet, deal 2 damage to the enemy leader and add a Depths of the Eld Tome to your hand.',
      jpn: '場のカード1枚を選ぶ。それを破壊。自分のアミュレットを選んだなら、相手のリーダーに2ダメージ。『天書の深淵』1枚を自分の手札に加える。',
    }),
  ]);
  assert.equal(rows.length, 1);
  assert.equal(rows[0].sentences.chs, 3);
  assert.equal(rows[0].sentences.eng, 2);
});

test('无条件词的卡牌不会进入 A 类候选', () => {
  const rows = screenSentenceSplit([
    card({chs: '抽取2张卡牌。', eng: 'Draw 2 cards.', jpn: '自分のデッキから2枚を引く。'}),
  ]);
  assert.equal(rows.length, 0);
});

test('日文出现「選んだなら」时进入 B 类候选', () => {
  const rows = screenSelectedCondition([
    card({
      chs: '选择自己的战场上的1张其他卡牌，若成功选择，则破坏该卡牌。对对手的战场上的随机1个随从造成2点伤害。',
      eng: 'Select another allied card on the field. If you selected one, destroy it and deal 2 damage to a random enemy follower.',
      jpn: '自分の場の他のカード1枚を選ぶ。選んだなら、それを破壊。相手の場のフォロワーからランダム1枚に2ダメージ。',
    }),
    card({
      chs: '抽取2张卡牌。',
      eng: 'Draw 2 cards.',
      jpn: '自分のデッキから2枚を引く。',
      id: 90000001,
      name: '白板',
    }),
  ]);
  assert.equal(rows.length, 1);
  assert.equal(rows[0].id, 90000000);
});

function writeRules(files) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'wbo-scope-'));
  for (const [name, body] of Object.entries(files)) {
    fs.writeFileSync(path.join(dir, name), body);
  }
  return dir;
}

test('scanRuleRequires 只挑出非法术卡里的 require', () => {
  const dir = writeRules({
    '10000001.wbo': `wbo 0.1.0;

card 10000001 {
    type follower;
    cost 2;
    stats 2/2;

    effect {
        fanfare {
            require target from oppo.field.followers;
            damage target 2;
        }
    }
}
`,
    '10000002.wbo': `wbo 0.1.0;

card 10000002 {
    type spell;
    cost 2;

    effect {
        require target from oppo.field.followers;
        damage target 2;
    }
}
`,
    '10000003.wbo': `wbo 0.1.0;

card 10000003 {
    type amulet;
    cost 4;

    effect {
        engage 0 {
            require ally from own.field.followers where form unevolved;
            destroy self;
        }
    }
}
`,
  });
  try {
    const rows = scanRuleRequires(['10000001.wbo', '10000002.wbo', '10000003.wbo'].map(name => path.join(dir, name)));
    assert.deepEqual(
      rows.map(row => [row.id, row.block]),
      [
        ['10000001', 'fanfare'],
        ['10000003', 'engage'],
      ],
    );
  } finally {
    fs.rmSync(dir, {recursive: true, force: true});
  }
});
