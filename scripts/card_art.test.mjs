import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { promisify } from 'node:util';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { importCardArt, planCardArt } from './import_card_art.mjs';

const run = promisify(execFile);
const pack = { format: 'wbos', kind: 'card-pack', contentHash: 'fixture', cards: [{ id: 12345670, cardType: 'follower' }, { id: 12345680, cardType: 'spell' }, { id: 90099910, cardType: 'follower' }] };
const metadata = [
  { card_id: 12345670, base_card_id: 12345670, card_style_id: 123456700, is_evolution: false, evolves_to: 12345671 },
  { card_id: 12345671, base_card_id: 12345670, card_style_id: 123456710, is_evolution: true },
  { card_id: 12345680, base_card_id: 12345680, card_style_id: 123456800, is_evolution: false },
];

test('art mapping follows source identity and texture presentation convention', () => {
  const result = planCardArt(pack, metadata);
  assert.deepEqual(result.planned.map(asset => [asset.cardId, asset.form, asset.textureId]), [[12345670, 'base', 123456700], [12345670, 'evolved', 123456701], [12345680, 'base', 123456800]]);
  assert.deepEqual(result.missing, [{ cardId: 90099910, reason: 'no_source_identity' }]);
  assert.throws(() => planCardArt(pack, [...metadata, metadata[0]]), /Duplicate/);
  assert.throws(() => planCardArt({ ...pack, cards: [pack.cards[0], pack.cards[0]] }, metadata), /duplicate/);
  assert.throws(() => planCardArt(pack, metadata.filter(card => !card.is_evolution)), /evolution identity/);
  assert.throws(() => planCardArt(pack, [{ ...metadata[0], card_style_id: 987654320 }, ...metadata.slice(1)]), /artwork identity/);
});

test('art import is incremental and repairs corrupted outputs without changing URLs', async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), 'wbo-art-test-'));
  try {
    const exportsRoot = path.join(root, 'exports');
    const webRoot = path.join(root, 'web');
    const packPath = path.join(root, 'cards.wbos');
    await mkdir(path.join(exportsRoot, 'analysis'), { recursive: true });
    await mkdir(path.join(exportsRoot, 'card-textures-resized/Main'), { recursive: true });
    await writeFile(packPath, JSON.stringify(pack));
    await writeFile(path.join(exportsRoot, 'analysis/cards_full.json'), JSON.stringify(metadata));
    const sources = planCardArt(pack, metadata).planned;
    for (const [n, asset] of sources.entries()) await run('magick', ['-size', '24x32', `xc:${['red', 'green', 'blue'][n]}`, path.join(exportsRoot, asset.source)]);
    const options = { packPath, exportsRoot, webRoot };
    assert.equal((await importCardArt(options)).converted, 3);
    const manifestPath = path.join(webRoot, 'public/assets/cards/manifest.json');
    const indexPath = path.join(webRoot, 'src/generated/card-art.json');
    const initial = await readFile(manifestPath, 'utf8');
    const index = await readFile(indexPath, 'utf8');
    assert.equal((await importCardArt(options)).converted, 0);
    assert.equal(await readFile(manifestPath, 'utf8'), initial);
    const first = JSON.parse(initial).assets[0];
    assert.deepEqual([first.width, first.height], [24, 32]);
    await writeFile(path.join(webRoot, 'public/assets/cards', first.file), 'broken');
    assert.equal((await importCardArt(options)).converted, 1);
    assert.equal(await readFile(manifestPath, 'utf8'), initial);
    await rm(path.join(exportsRoot, sources[2].source));
    await assert.rejects(importCardArt(options), /ENOENT/);
    assert.equal(await readFile(indexPath, 'utf8'), index);
    assert.equal(await readFile(manifestPath, 'utf8'), initial);
  } finally { await rm(root, { recursive: true, force: true }); }
});

test('checked-in art index resolves every manifest entry and content hash', async () => {
  const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../web');
  const manifest = JSON.parse(await readFile(path.join(webRoot, 'public/assets/cards/manifest.json'), 'utf8'));
  const index = JSON.parse(await readFile(path.join(webRoot, 'src/generated/card-art.json'), 'utf8'));
  let indexed = 0;
  for (const forms of Object.values(index)) indexed += Object.keys(forms).length;
  assert.equal(indexed, manifest.assets.length);
  for (const asset of manifest.assets) {
    assert.equal(index[asset.cardId][asset.form], `/assets/cards/${asset.file}`);
    const bytes = await readFile(path.join(webRoot, 'public/assets/cards', asset.file));
    assert.equal(createHash('sha256').update(bytes).digest('hex'), asset.sha256);
    assert.equal(bytes.toString('ascii', 0, 4), 'RIFF');
    assert.equal(bytes.toString('ascii', 8, 12), 'WEBP');
    assert(asset.width > 0 && asset.height > 0);
  }
  for (const missing of manifest.missing) assert.equal(index[missing.cardId], undefined);
});
