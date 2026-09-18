#!/usr/bin/env node
// 把 WBArts 的主界面插图（home illustration）原样打进客户端素材目录：
// 背景 PNG、Spine 的 skel/atlas/图集 PNG 与缩略图都**逐字节复制，不做任何压缩或转码**。
// 同时生成 web/public/assets/home/index.json 作为清单；前端优先读它，
// 读不到时才退回规则服务的 /api/illustrations。
//
//   node scripts/bundle_illustrations.mjs --data ../WBArts/data --out web/public/assets/home
//   node scripts/bundle_illustrations.mjs --ids hi_1001,hi_1002
//   node scripts/bundle_illustrations.mjs --types home,battle,switch
//   node scripts/bundle_illustrations.mjs --dry-run
//
// 只复制素材；把 675 MB 的立绘提交进 git 并不合适，所以默认把这些目录写进
// .gitignore（见脚本末尾提示），打包客户端时在构建机上跑一次即可。
import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import process from "node:process";

function parseArgs(argv) {
  const options = { data: "../WBArts/data", out: "web/public/assets/home", types: ["home"], ids: null, dryRun: false, force: false };
  for (let index = 0; index < argv.length; index++) {
    const arg = argv[index];
    const [key, inline] = arg.split("=");
    const value = inline ?? argv[index + 1];
    const take = () => { if (inline === undefined) index++; return value; };
    if (key === "--data") options.data = take();
    else if (key === "--out") options.out = take();
    else if (key === "--types") options.types = take().split(",").map((item) => item.trim()).filter(Boolean);
    else if (key === "--ids") options.ids = take().split(",").map((item) => item.trim()).filter(Boolean);
    else if (key === "--dry-run") { options.dryRun = true; }
    else if (key === "--force") { options.force = true; }
    else if (key === "--help" || key === "-h") { options.help = true; }
    else throw new Error(`未知参数 ${arg}`);
  }
  return options;
}

function sha256(file) {
  return createHash("sha256").update(fs.readFileSync(file)).digest("hex");
}

/** 复制单个文件（逐字节），返回大小与哈希。 */
function copyVerbatim(from, to) {
  fs.mkdirSync(path.dirname(to), { recursive: true });
  fs.copyFileSync(from, to);
  const stat = fs.statSync(to);
  return { bytes: stat.size, sha256: sha256(to) };
}

function readJson(file) {
  return JSON.parse(fs.readFileSync(file, "utf8"));
}

// 索引里的路径以 data/ 开头（相对于仓库根），这里统一转成相对 --data 的路径。
function dataPath(dataDir, value) {
  if (!value) return "";
  const relative = String(value).replace(/^data[\\/]/, "");
  return path.join(dataDir, relative);
}

function main() {
  const options = parseArgs(process.argv.slice(2));
  if (options.help) {
    console.log("用法: node scripts/bundle_illustrations.mjs [--data DIR] [--out DIR] [--types home,battle] [--ids hi_1001] [--dry-run] [--force]");
    return 0;
  }
  const dataDir = path.resolve(options.data);
  const outDir = path.resolve(options.out);
  const indexFile = path.join(dataDir, "home_illust_index.json");
  if (!fs.existsSync(indexFile)) {
    console.error(`找不到 ${indexFile}：请用 --data 指向 WBArts 的 data 目录`);
    return 1;
  }
  const index = readJson(indexFile);
  const entries = index.filter((item) => {
    if (options.ids) return options.ids.includes(item.id);
    return options.types.includes(item.type || "home");
  });
  if (!entries.length) {
    console.error("没有匹配的插图");
    return 1;
  }
  const manifest = [];
  let totalBytes = 0;
  let copied = 0;
  for (const item of entries) {
    const dir = path.join(dataDir, "home-illustration", item.id);
    const configFile = path.join(dir, "config.json");
    const skel = dataPath(dataDir, item.spine?.skel);
    const atlas = dataPath(dataDir, item.spine?.atlas);
    const texture = dataPath(dataDir, item.spine?.png);
    const background = dataPath(dataDir, item.backgrounds?.[0]);
    const thumbnail = dataPath(dataDir, item.thumbnail);
    for (const [label, file] of Object.entries({ skel, atlas, texture, background })) {
      if (!file || !fs.existsSync(file)) {
        console.error(`${item.id}: 缺少 ${label}（${file || "未在索引中声明"}）`);
        return 1;
      }
    }
    const config = fs.existsSync(configFile) ? readJson(configFile) : {};
    const record = {
      id: item.id,
      name: item.names?.chs || config.character_name || item.id,
      type: item.type || "home",
      source: "bundled",
      skel: `/assets/home/${item.id}/${path.basename(skel)}`,
      atlas: `/assets/home/${item.id}/${path.basename(atlas)}`,
      texture: `/assets/home/${item.id}/${path.basename(texture)}`,
      background: `/assets/home/${item.id}/${path.basename(background)}`,
      thumbnail: thumbnail && fs.existsSync(thumbnail) ? `/assets/home/${item.id}/thumb.png` : "",
      idleAnimation: item.idleAnimation || config.idle_animation || "idle",
      tapAnimations: item.tapAnimations || config.tap_animations || [],
      blendTimes: item.blendTimes || config.blend_times || [],
      defaultMix: Number(item.defaultMix ?? config.default_mix ?? 0.2),
      skeletonScale: Number(item.skeletonScale ?? config.skeleton_scale ?? 0.01),
      prefabScale: Number(item.prefabScale ?? config.prefab_transforms?.prefab_scale ?? 1),
      aspectLayouts: item.aspectLayouts || {},
      files: {},
    };
    if (options.dryRun) {
      for (const [label, file] of Object.entries({ skel, atlas, texture, background })) {
        const bytes = fs.statSync(file).size;
        record.files[label] = { bytes, sha256: sha256(file) };
        totalBytes += bytes;
      }
      manifest.push(record);
      continue;
    }
    for (const [label, file] of Object.entries({ skel, atlas, texture, background })) {
      const target = path.join(outDir, item.id, path.basename(file));
      if (options.force || !fs.existsSync(target) || sha256(target) !== sha256(file)) {
        record.files[label] = copyVerbatim(file, target);
        copied++;
      } else {
        record.files[label] = { bytes: fs.statSync(target).size, sha256: record.files[label]?.sha256 ?? sha256(target) };
        record.files[label].sha256 = sha256(target);
      }
      totalBytes += record.files[label].bytes;
    }
    if (thumbnail && fs.existsSync(thumbnail)) {
      const target = path.join(outDir, item.id, "thumb.png");
      if (options.force || !fs.existsSync(target) || sha256(target) !== sha256(thumbnail)) {
        record.files.thumbnail = copyVerbatim(thumbnail, target);
        copied++;
      }
    }
    manifest.push(record);
  }
  const megabytes = (totalBytes / 1048576).toFixed(1);
  if (options.dryRun) {
    console.log(`将打包 ${manifest.length} 张主界面插图，合计 ${megabytes} MB（未复制任何文件）`);
    for (const record of manifest.slice(0, 5)) {
      const size = Object.values(record.files).reduce((sum, file) => sum + file.bytes, 0);
      console.log(`  ${record.id} ${record.name} ${(size / 1048576).toFixed(1)} MB`);
    }
    if (manifest.length > 5) console.log(`  … 其余 ${manifest.length - 5} 张`);
    return 0;
  }
  fs.mkdirSync(outDir, { recursive: true });
  const manifestFile = path.join(outDir, "index.json");
  fs.writeFileSync(manifestFile, JSON.stringify({ version: 1, generatedFrom: options.data, items: manifest }, null, 2) + "\n");
  console.log(`已打包 ${manifest.length} 张主界面插图（${megabytes} MB，逐字节复制，无压缩）`);
  console.log(`新复制/更新 ${copied} 个文件；清单写入 ${path.relative(process.cwd(), manifestFile)}`);
  console.log("提示：素材目录默认不进 git；打包客户端（Wails/Electron）时在构建机上跑一次本脚本即可。");
  return 0;
}

try {
  process.exitCode = main();
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
}
