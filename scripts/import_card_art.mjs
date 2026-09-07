import { createHash } from 'node:crypto';
import { execFile } from 'node:child_process';
import { mkdir, readFile, rename, rm, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';

const run = promisify(execFile);
const sha256 = data => createHash('sha256').update(data).digest('hex');
const recipe = { format: 'webp', width: 636, height: 768, quality: 84, method: 5 };

export function planCardArt(pack, metadata) {
  if (pack.format !== 'wbos' || pack.kind !== 'card-pack' || !Array.isArray(pack.cards) || !Array.isArray(metadata)) {
    throw new Error('Expected a compiled WBOS card pack and exported card metadata');
  }
  const records = new Map();
  for (const record of metadata) {
    if (records.has(record.card_id)) throw new Error(`Duplicate source card ${record.card_id}`);
    records.set(record.card_id, record);
  }
  const cards = [...pack.cards].sort((a, b) => a.id - b.id);
  const ids = new Set();
  const planned = [];
  const missing = [];
  for (const card of cards) {
    if (!Number.isSafeInteger(card.id) || card.id < 10000000 || card.id > 99999999 || ids.has(card.id)) throw new Error('Invalid or duplicate card ID');
    ids.add(card.id);
    const record = records.get(card.id);
    if (!record) { missing.push({ cardId: card.id, reason: 'no_source_identity' }); continue; }
    if (!Number.isSafeInteger(record.card_style_id) || record.card_style_id !== card.id * 10 || record.is_evolution || record.base_card_id !== card.id) {
      throw new Error(`Unsupported base artwork identity for ${card.id}`);
    }
    const forms = ['base'];
    if (card.cardType === 'follower') {
      const evolved = records.get(record.evolves_to);
      if (!evolved?.is_evolution || evolved.base_card_id !== card.id || evolved.card_id !== card.id + 1) throw new Error(`Missing evolution identity for ${card.id}`);
      forms.push('evolved');
    }
    for (const form of forms) {
      // Texture exports use the presentation ID: base style + 1 for evolved art.
      const textureId = record.card_style_id + Number(form === 'evolved');
      const category = String(textureId).startsWith('9') ? 'Token' : String(textureId).startsWith('8') ? 'Special' : 'Main';
      planned.push({ cardId: card.id, form, textureId, source: `card-textures-resized/${category}/${textureId}.png` });
    }
  }
  return { planned, missing };
}

export async function importCardArt({ packPath, exportsRoot, webRoot }) {
  const pack = JSON.parse(await readFile(packPath, 'utf8'));
  const metadata = JSON.parse(await readFile(path.join(exportsRoot, 'analysis/cards_full.json'), 'utf8'));
  const { planned, missing } = planCardArt(pack, metadata);
  // Validate all requested sources before publishing any new index.
  const sources = new Map();
  for (const asset of planned) sources.set(asset.source, sha256(await readFile(path.join(exportsRoot, asset.source))));
  const output = path.join(webRoot, 'public/assets/cards');
  const indexPath = path.join(webRoot, 'src/generated/card-art.json');
  await mkdir(output, { recursive: true });
  await mkdir(path.dirname(indexPath), { recursive: true });
  let previous;
  try { previous = JSON.parse(await readFile(path.join(output, 'manifest.json'), 'utf8')); }
  catch (error) { if (error.code !== 'ENOENT') throw error; }
  const cached = new Map((previous?.assets ?? []).map(asset => [asset.source, asset]));
  const assets = [];
  const index = {};
  let converted = 0;
  for (const asset of planned) {
    const sourceSha256 = sources.get(asset.source);
    const old = cached.get(asset.source);
    let entry;
    if (JSON.stringify(previous?.recipe) === JSON.stringify(recipe) && old?.sourceSha256 === sourceSha256 && /^\d{9}-[0-9a-f]{16}\.webp$/.test(old.file)) {
      try {
        if (sha256(await readFile(path.join(output, old.file))) === old.sha256) entry = { ...asset, sourceSha256, file: old.file, sha256: old.sha256, width: old.width, height: old.height };
      } catch (error) { if (error.code !== 'ENOENT') throw error; }
    }
    if (!entry) {
      const temporary = path.join(output, `.${asset.textureId}-${process.pid}.webp`);
      try {
        const { stdout } = await run('magick', [path.join(exportsRoot, asset.source), '-limit', 'thread', '1', '-resize', `${recipe.width}x${recipe.height}>`, '-strip', '-quality', String(recipe.quality), '-define', `webp:method=${recipe.method}`, '-write', temporary, '-format', '%w %h', 'info:']);
        const [width, height] = stdout.trim().split(/\s+/).map(Number);
        if (!(width > 0 && height > 0 && width <= recipe.width && height <= recipe.height)) throw new Error(`Invalid converted dimensions for ${asset.textureId}`);
        const hash = sha256(await readFile(temporary));
        const file = `${asset.textureId}-${hash.slice(0, 16)}.webp`;
        await rename(temporary, path.join(output, file));
        entry = { ...asset, sourceSha256, file, sha256: hash, width, height };
        converted++;
      } finally { await rm(temporary, { force: true }); }
    }
    assets.push(entry);
    index[asset.cardId] ??= {};
    index[asset.cardId][asset.form] = `/assets/cards/${entry.file}`;
  }
  const manifest = { version: 1, cardPackHash: pack.contentHash, recipe, assets, missing };
  for (const [file, value] of [[path.join(output, 'manifest.json'), manifest], [indexPath, index]]) {
    const temporary = `${file}.${process.pid}.tmp`;
    await writeFile(temporary, JSON.stringify(value, null, 2) + '\n');
    await rename(temporary, file);
  }
  return { cards: Object.keys(index).length, assets: assets.length, missing, converted };
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [packPath, exportsRoot, webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../web')] = process.argv.slice(2);
  if (!packPath || !exportsRoot || process.argv.length > 5) {
    console.error('Usage: node scripts/import_card_art.mjs <cards.wbos> <asset-exports> [web-root]');
    process.exitCode = 1;
  } else {
    try { console.log(JSON.stringify(await importCardArt({ packPath, exportsRoot, webRoot }))); }
    catch (error) { console.error(error.message); process.exitCode = 1; }
  }
}
