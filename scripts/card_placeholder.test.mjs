import assert from 'node:assert/strict';
import {test} from 'node:test';

import {isPlaceholder} from './card_placeholder.mjs';

const card = effect => `wbo 0.1.0;

card 12345678 {
    type follower;
    cost 1;
    stats 1/1;

    effect {
${effect}
    }

    meta { pack 10000; class neutral; rarity bronze; }
}
`;

test('an effect block holding only unplayable is a placeholder', () => {
  assert.equal(isPlaceholder(card('        unplayable;')), true);
  assert.equal(isPlaceholder(card('        unplayable;\n<< 未实现')), true);
});

test('a genuinely unplayable card keeps counting as implemented', () => {
  assert.equal(
    isPlaceholder(
      card(`        unplayable;
        fusion material from own.hand where type amulet and trait artifact {
            transform self into card 90072110 preserving materials;
        }`),
    ),
    false,
  );
});

test('implemented effects and empty effects are not placeholders', () => {
  assert.equal(isPlaceholder(card('        ward;')), false);
  assert.equal(isPlaceholder(card('')), false);
});

test('the first effect block decides even when a crest follows', () => {
  const source = `${card('        unplayable;')}
    crest {
        countdown 2;
        when own turn ends { draw 1; }
    }
`;
  assert.equal(isPlaceholder(source), true);
});
