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
export type BackgroundImage = { url: string; x: number; y: number; width: number; height: number };

const toNum = (value: unknown, fallback: number) => {
  const n = Number(value);
  return Number.isFinite(n) ? n : fallback;
};

// 按容器宽高比挑一套布局，与 WBArts 的 getLayout 一致（16:9 优先）。
export function layoutFor(config: HomeIllustration, aspect: number) {
  const layouts = config.aspectLayouts || {};
  const candidates = Object.entries(layouts);
  if (layouts["16:9"]) return layouts["16:9"];
  if (layouts["default"]) return layouts["default"];
  let best = candidates[0]?.[1] || { x: 0, y: 0, scale_x: 1, scale_y: 1 };
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
// 再按布局位移居中。
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
  const width = 19.2 / scaleX;
  const height = 10.8 / scaleY;
  return { x: -unityX / scaleX - width / 2, y: -unityY / scaleY - height / 2, width, height };
}

// 背景贴图：1:1 的图按"填满相机（含 spine-player 默认 10% 内边距）"换算成
// spine 世界坐标，和 WBArts 的做法一致。
export function computeBackgroundImage(config: HomeIllustration, viewport: Viewport): BackgroundImage {
  const pad = 0.1;
  const camW = viewport.width * (1 + 2 * pad);
  const camH = viewport.height * (1 + 2 * pad);
  const camX = viewport.x - viewport.width * pad;
  const camY = viewport.y - viewport.height * pad;
  const bgW = camW;
  const bgH = bgW;
  return { url: config.background, x: camX, y: camY - (bgH - camH) / 2, width: bgW, height: bgH };
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
  const tapIndex = useRef(0);

  useEffect(() => {
    const element = host.current;
    if (!element) return;
    let disposed = false;
    const runtime = (window as unknown as {
      spine?: { SpinePlayer?: new (host: HTMLElement, options: Record<string, unknown>) => SpinePlayerLike };
    }).spine;
    if (!runtime?.SpinePlayer) { setFailed(true); return; }
    const aspect = element.clientWidth && element.clientHeight ? element.clientWidth / element.clientHeight : ratio;
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
      viewport: { x: viewport.x, y: viewport.y, width: viewport.width, height: viewport.height },
      success: (created: SpinePlayerLike) => {
        if (disposed) return;
        player.current = created;
        if (created.animationState?.data) created.animationState.data.defaultMix = config.defaultMix;
      },
      error: () => { if (!disposed) setFailed(true); },
    });
    player.current = instance;
    return () => { disposed = true; player.current?.dispose?.(); player.current = null; };
  }, [config, ratio]);

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
      className={`home-illustration ${className}`.trim()}
      onPointerDown={playTap}
      data-illustration={config.id}
      aria-label={`${config.name} 主界面插画`}
    >
      {failed && <img src={config.background} alt="" className="home-illustration-fallback" />}
    </div>
  );
}
