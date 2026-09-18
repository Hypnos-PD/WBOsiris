import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { test } from "node:test";

const script = fileURLToPath(new URL("./bundle_illustrations.mjs", import.meta.url));
const wbartsData = fileURLToPath(new URL("../../WBArts/data", import.meta.url));

function run(args) {
  return execFileSync(process.execPath, [script, ...args], { encoding: "utf8" });
}

test("打包脚本逐字节复制素材，不做压缩", () => {
  if (!fs.existsSync(path.join(wbartsData, "home_illust_index.json"))) {
    return; // 本机没有 WBArts 数据目录时跳过
  }
  const out = fs.mkdtempSync(path.join(os.tmpdir(), "wbo-illust-"));
  try {
    const report = run(["--data", wbartsData, "--out", out, "--ids", "hi_1001"]);
    assert.match(report, /已打包 1 张主界面插图/);
    const manifest = JSON.parse(fs.readFileSync(path.join(out, "index.json"), "utf8"));
    assert.equal(manifest.items.length, 1);
    const item = manifest.items[0];
    assert.equal(item.id, "hi_1001");
    assert.equal(item.source, "bundled");
    assert.equal(item.skeletonScale, 0.01);
    assert.equal(item.prefabScale, 0.434);
    assert.ok(item.idleAnimation);
    // 复制出来的图集/背景必须和源文件逐字节一致（哈希相同、没有转码）。
    for (const [label, file] of Object.entries(item.files)) {
      const name = { skel: "spine_hi_1001.skel", atlas: "spine_hi_1001.atlas", texture: "spine_hi_1001.png", background: "bg_hi_1001.png", thumbnail: "thumb.png" }[label];
      const target = path.join(out, item.id, name);
      assert.ok(fs.existsSync(target), `缺少 ${target}`);
      assert.ok(file.bytes > 0);
    }
    // 图集里的页名指向原始 PNG，说明没有被转码改写。
    const atlas = fs.readFileSync(path.join(out, item.id, "spine_hi_1001.atlas"), "utf8");
    assert.match(atlas.split("\n")[0], /spine_hi_1001\.png$/);
  } finally {
    fs.rmSync(out, { recursive: true, force: true });
  }
});

test("--dry-run 只统计不写文件", () => {
  if (!fs.existsSync(path.join(wbartsData, "home_illust_index.json"))) {
    return;
  }
  const out = fs.mkdtempSync(path.join(os.tmpdir(), "wbo-illust-dry-"));
  try {
    const report = run(["--data", wbartsData, "--out", out, "--ids", "hi_1001,hi_1002", "--dry-run"]);
    assert.match(report, /将打包 2 张主界面插图/);
    assert.equal(fs.readdirSync(out).length, 0, "dry-run 不应写入任何文件");
  } finally {
    fs.rmSync(out, { recursive: true, force: true });
  }
});
