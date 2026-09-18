#!/usr/bin/env node
// 构建 web/ 前端并把产物放到 desktop/frontend/dist，供 Wails 嵌入。
//
//   node scripts/release/build-frontend.mjs            # vite build + 同步
//   node scripts/release/build-frontend.mjs --skip-build   # 只同步已有 dist
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(fileURLToPath(new URL("../..", import.meta.url)));
const web = path.join(root, "web");
const source = path.join(web, "dist");
const target = path.join(root, "desktop", "frontend", "dist");
const skipBuild = process.argv.includes("--skip-build");
// 默认不把 40 张主界面立绘（约 675 MB）塞进二进制：它们由发布包放在
// 可执行文件旁的 assets/ 目录里，客户端直接提供。要单文件自包含时才加 --with-illustrations。
const withIllustrations = process.argv.includes("--with-illustrations");

if (!skipBuild) {
  console.log("构建前端（vite build）…");
  execFileSync("npm", ["run", "build"], { cwd: web, stdio: "inherit" });
}
if (!fs.existsSync(path.join(source, "index.html"))) {
  throw new Error(`前端产物不存在：${source}/index.html`);
}

fs.rmSync(target, { recursive: true, force: true });
fs.mkdirSync(target, { recursive: true });
fs.cpSync(source, target, {
  recursive: true,
  filter: (from) => {
    if (withIllustrations) return true;
    const relative = path.relative(source, from).split(path.sep).join("/");
    if (!relative.startsWith("assets/home/")) return true;
    // 保留内置默认立绘（hi_1001/）与它的参数；其余立绘留给发布包。
    const rest = relative.slice("assets/home/".length);
    if (rest === "" ) return true;
    const [head] = rest.split("/");
    if (head === "hi_1001" || head === "hi_1001.json") return true;
    return false;
  },
});

const size = (dir) => fs.readdirSync(dir, { withFileTypes: true }).reduce((total, entry) => {
  const full = path.join(dir, entry.name);
  return total + (entry.isDirectory() ? size(full) : fs.statSync(full).size);
}, 0);
console.log(`已同步前端到 ${path.relative(root, target)}（${(size(target) / 1048576).toFixed(1)} MB）`);
