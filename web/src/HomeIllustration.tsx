import { useEffect, useRef, useState } from "react";

// 主界面插图（home illustration）的合成参数，来自 WBArts 的
// data/home-illustration/hi_1001（index + config）。数值直接沿用，方便对照。
export type HomeIllustration = {
  id: string;
  name: string;
  skeletonScale: number;
  prefabScale: number;
  idleAnimation: string;
  tapAnimations: string[];
  blendTimes: number[];
  defaultMix: number;
  aspectLayouts: Record<string, { x?: number; y?: number; scale_x?: number; scale_y?: number }>;
  skel: string;
  atlas: string;
  background: string;
  source?: string;
};

export type Viewport = { x: number; y: number; width: number; height: number };

// 见 computeViewport 的说明：覆盖计算出来后额外放大一点，避免播放器 Fit 出来的缝。
const coverSafety = 1.06;
export type BackgroundImage = { url: string; x: number; y: number; width: number; height: number };

const toNum = (value: unknown, fallback: number) => {
  const n = Number(value);
  return Number.isFinite(n) ? n : fallback;
};

// 按容器宽高比挑最接近的一套布局：游戏里 aspectLayouts 就是给不同屏幕比例用的
// （4:3 / 16:9 / 21:10 / 21:9）。没有布局数据时退回 16:9 或第一套。
export function layoutFor(config: HomeIllustration, aspect: number) {
  const layouts = config.aspectLayouts || {};
  const candidates = Object.entries(layouts);
  let best = layouts["16:9"] || layouts["default"] || candidates[0]?.[1] || { x: 0, y: 0, scale_x: 1, scale_y: 1 };
  if (candidates.length < 2) return best;
  let bestDelta = Infinity;
  for (const [key, value] of candidates) {
    const [w, h] = key.split(":").map(Number);
    if (!w || !h) continue;
    const delta = Math.abs(w / h - aspect);
    if (delta < bestDelta) { bestDelta = delta; best = value; }
  }
  return best;
}

// 相机窗口：Unity 世界单位 19.2×10.8 除以 skeletonScale × prefabScale × 布局缩放，
// 再按布局位移居中；最后按容器比例把窗口**扩到铺满**（cover），
// 这样窄屏/带鱼屏都不会出现黑边（WBArts 的页面容器固定 16:9，所以那边看不出差别）。
// hi_1001 在 16:9 + 默认布局下的结果（与 WBArts 的计算一致，可用来对表）：
//   viewport   { x: -1546.84, y: -870.10, width: 3093.68, height: 1740.20 }
//   background { x: -1856.21, y: -1856.21, width: 3712.42, height: 3712.42 }
export function computeViewport(config: HomeIllustration, aspect: number, yBiasPx = 0): Viewport {
  const layout = layoutFor(config, aspect);
  const sx = toNum(layout?.scale_x, 1);
  const sy = toNum(layout?.scale_y, sx);
  const scaleX = toNum(config.skeletonScale, 0.01) * toNum(config.prefabScale, 1) * sx;
  const scaleY = toNum(config.skeletonScale, 0.01) * toNum(config.prefabScale, 1) * sy;
  const unityX = toNum(layout?.x, 0);
  const unityY = toNum(layout?.y, 0) + toNum(yBiasPx, 0) / 100;
  let width = 19.2 / scaleX;
  let height = 10.8 / scaleY;
  const design = height > 0 ? width / height : 16 / 9;
  if (aspect > 0 && Number.isFinite(aspect)) {
    if (aspect > design) width = height * aspect;
    else height = width / aspect;
  }
  // 安全放大：spine-player 会按"带 10% 内边距的视口"做 Fit，实测容器上下仍会剩一条
  // 约 3% 的缝；放大 6% 把它推到画面外，构图中心不变。
  width *= coverSafety;
  height *= coverSafety;
  return { x: -unityX / scaleX - width / 2, y: -unityY / scaleY - height / 2, width, height };
}

// 背景贴图：1:1 的图按"填满相机（含 spine-player 默认 10% 内边距）"换算成
// spine 世界坐标；图是方的，所以边长取相机长短边里更大的那个，保证两个方向都盖住。
export function computeBackgroundImage(config: HomeIllustration, viewport: Viewport): BackgroundImage {
  const pad = 0.1;
  const camW = viewport.width * (1 + 2 * pad);
  const camH = viewport.height * (1 + 2 * pad);
  const camX = viewport.x - viewport.width * pad;
  const camY = viewport.y - viewport.height * pad;
  const side = Math.max(camW, camH);
  return {
    url: config.background,
    x: camX + camW / 2 - side / 2,
    y: camY + camH / 2 - side / 2,
    width: side,
    height: side,
  };
}

type SpinePlayerLike = {
  animationState?: {
    setAnimation?: (track: number, name: string, loop: boolean) => void;
    addAnimation?: (track: number, name: string, loop: boolean, delay: number) => void;
    data?: { defaultMix?: number };
  };
  skeleton?: { setSkinByName?: (name: string) => void };
  dispose?: () => void;
};

// 一块"背景 + Spine 立绘"的主界面插图；点击会播一次 tap 动画（和游戏里一样）。
export function HomeIllustrationView({ config, className = "", ratio = 16 / 9, interactive = true }: {
  config: HomeIllustration;
  className?: string;
  ratio?: number;
  interactive?: boolean;
}) {
  const host = useRef<HTMLDivElement>(null);
  const player = useRef<SpinePlayerLike | null>(null);
  const [failed, setFailed] = useState(false);
  // spine 运行时会先把"加载中"的 logo + 转圈画进 canvas，所以加载完成前不显示 canvas。
  const [ready, setReady] = useState(false);
  // 窗口尺寸变化时按新的宽高比重新取景（防抖后重建播放器）。
  const [tick, setTick] = useState(0);
  const tapIndex = useRef(0);

  // 必须等元素真的有尺寸再建播放器：挂载瞬间 clientWidth/Height 可能都是 0，
  // 那时算出来的相机比例会退回 16:9，画面就被 letterbox 出黑边。
  useEffect(() => {
    const element = host.current;
    if (!element) return;
    let timer: number | undefined;
    let disposed = false;
    const schedule = () => {
      if (disposed) return;
      window.clearTimeout(timer);
      timer = window.setTimeout(() => setTick((value) => value + 1), 250);
    };
    const observer = new ResizeObserver(schedule);
    observer.observe(element);
    window.addEventListener("resize", schedule);
    return () => {
      disposed = true;
      observer.disconnect();
      window.removeEventListener("resize", schedule);
      window.clearTimeout(timer);
    };
  }, []);

  useEffect(() => {
    const element = host.current;
    if (!element) return;
    let disposed = false;
    setReady(false);
    const runtime = (window as unknown as {
      spine?: { SpinePlayer?: new (host: HTMLElement, options: Record<string, unknown>) => SpinePlayerLike };
    }).spine;
    if (!runtime?.SpinePlayer) { setFailed(true); return; }
    const width = element.clientWidth;
    const height = element.clientHeight;
    // 还没有布局就先不建，等 ResizeObserver 触发一次重建。
    if (!width || !height) return;
    const aspect = width / height;
    const viewport = computeViewport(config, aspect);
    const instance = new runtime.SpinePlayer(element, {
      skelUrl: config.skel,
      atlasUrl: config.atlas,
      animation: config.idleAnimation || "idle",
      skin: "JP",
      backgroundImage: computeBackgroundImage(config, viewport),
      alpha: true,
      showControls: false,
      premultipliedAlpha: true,
      backgroundColor: "#00000000",
      // 相机窗口已经按容器比例算好，这里让它铺满画布（不要 Fit 的黑边）。
      resizeMode: "Stretch",
      viewport: { x: viewport.x, y: viewport.y, width: viewport.width, height: viewport.height },
      success: (created: SpinePlayerLike) => {
        if (disposed) return;
        player.current = created;
        if (created.animationState?.data) created.animationState.data.defaultMix = config.defaultMix;
        setReady(true);
      },
      error: () => { if (!disposed) setFailed(true); },
    });
    player.current = instance;
    return () => { disposed = true; player.current?.dispose?.(); player.current = null; };
  }, [config, ratio, tick]);

  const playTap = () => {
    if (!interactive) return;
    const taps = config.tapAnimations || [];
    const state = player.current?.animationState;
    if (!taps.length || !state?.setAnimation) return;
    const name = taps[tapIndex.current % taps.length];
    const mix = config.blendTimes?.[tapIndex.current % taps.length] ?? 0.25;
    tapIndex.current += 1;
    state.setAnimation(0, config.idleAnimation || "idle", true);
    state.addAnimation?.(0, name, false, mix);
  };

  return (
    <div
      ref={host}
      className={`home-illustration ${ready ? "ready" : "loading"} ${className}`.trim()}
      onPointerDown={playTap}
      data-illustration={config.id}
      aria-label={`${config.name} 主界面插画`}
    >
      {failed && <img src={config.background} alt="" className="home-illustration-fallback" />}
    </div>
  );
}
