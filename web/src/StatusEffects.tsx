import { useEffect, useRef } from "react";
import "./status-effects.css";

export type StatusEffect =
  | "abilityProtected"
  | "aura"
  | "barrier"
  | "damageReduction"
  | "hold"
  | "intimidate"
  | "selfDestruction"
  | "stealth";

function QuarterEffect({
  kind,
  src,
}: {
  kind: "ability-protected" | "aura" | "barrier";
  src: string;
}) {
  return (
    <span className={`status-quarter status-${kind}`}>
      {[0, 1, 2, 3].map((index) => (
        <i
          className={`status-quarter-${index}`}
          key={index}
          style={{ backgroundImage: `url(${src})` }}
        />
      ))}
    </span>
  );
}

function IntimidateEffect() {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const impact = new Image(),
      fire = new Image();
    let loaded = 0,
      cancelled = false;
    const draw = () => {
      if (++loaded !== 2 || cancelled) return;
      const context = canvas.getContext("2d");
      if (!context) return;
      const { width, height } = canvas;
      const layer = document.createElement("canvas");
      layer.width = width;
      layer.height = height;
      const effect = layer.getContext("2d");
      if (!effect) return;
      const points = [
        [width / 2, 9],
        [width - 9, height / 2],
        [width / 2, height - 9],
        [9, height / 2],
      ];
      const outer = new Path2D();
      outer.moveTo(points[0][0], points[0][1]);
      for (let index = 1; index < points.length; index++)
        outer.lineTo(points[index][0], points[index][1]);
      outer.closePath();
      const inset = 18;
      const inner = [
        [width / 2, 9 + inset],
        [width - 9 - inset, height / 2],
        [width / 2, height - 9 - inset],
        [9 + inset, height / 2],
      ];
      const innerPath = new Path2D();
      innerPath.moveTo(inner[0][0], inner[0][1]);
      for (let index = 1; index < inner.length; index++)
        innerPath.lineTo(inner[index][0], inner[index][1]);
      innerPath.closePath();
      effect.save();
      effect.clip(outer);
      effect.globalAlpha = 0.96;
      effect.drawImage(fire, -20, -20, width + 40, height + 40);
      effect.restore();
      effect.globalCompositeOperation = "lighter";
      for (let index = 0; index < 4; index++) {
        const from = points[index],
          to = points[(index + 1) % 4],
          dx = to[0] - from[0],
          dy = to[1] - from[1],
          length = Math.hypot(dx, dy);
        effect.save();
        effect.translate((from[0] + to[0]) / 2, (from[1] + to[1]) / 2);
        effect.rotate(Math.atan2(dy, dx));
        effect.globalAlpha = 0.46;
        effect.drawImage(impact, -length * 0.56, -31, length * 1.12, 62);
        effect.globalCompositeOperation = "destination-out";
        effect.globalAlpha = 0.82;
        effect.fillRect(-length * 0.58, -2.5, length * 1.16, 5);
        effect.restore();
      }
      effect.globalCompositeOperation = "destination-in";
      effect.fillStyle = "#fff";
      effect.fill(outer);
      effect.globalCompositeOperation = "destination-out";
      effect.fillStyle = "#000";
      effect.fill(innerPath);
      effect.globalCompositeOperation = "source-in";
      effect.fillStyle = "rgb(76, 238, 126)";
      effect.fillRect(0, 0, width, height);
      effect.globalCompositeOperation = "source-atop";
      effect.globalAlpha = 0.22;
      effect.fillStyle = "#c1ffd1";
      effect.fillRect(0, 0, width, height);
      context.clearRect(0, 0, width, height);
      context.globalCompositeOperation = "screen";
      context.drawImage(layer, 0, 0);
    };
    impact.onload = draw;
    fire.onload = draw;
    impact.src =
      "/assets/status-prefabs/stt_loop_unattacked_1/ef_impact009.png";
    fire.src =
      "/assets/status-prefabs/stt_loop_unattacked_1/ef_fire014_tra.png";
    return () => {
      cancelled = true;
    };
  }, []);
  return (
    <canvas
      className="status-intimidate"
      ref={canvasRef}
      width="280"
      height="356"
    />
  );
}

export function StatusEffects({ statuses }: { statuses: StatusEffect[] }) {
  return (
    <span className="status-effects" aria-hidden="true">
      {statuses.includes("aura") && (
        <QuarterEffect
          kind="aura"
          src="/assets/status-prefabs/stt_loop_unselected_1/ef_guard004_tra.png"
        />
      )}
      {statuses.includes("barrier") && (
        <QuarterEffect
          kind="barrier"
          src="/assets/status-prefabs/stt_loop_damagecut_1/ef_ring001.png"
        />
      )}
      {statuses.includes("abilityProtected") && (
        <QuarterEffect
          kind="ability-protected"
          src="/assets/status-prefabs/stt_loop_damagecut_1/ef_ring001.png"
        />
      )}
      {statuses.includes("damageReduction") && (
        <span className="status-damage-reduction" title="受到的伤害减少" aria-label="受到的伤害减少" />
      )}
      {statuses.includes("intimidate") && <IntimidateEffect />}
      {statuses.includes("stealth") && <span className="status-stealth" />}
      {statuses.includes("hold") && (
        <img
          className="status-whole status-hold"
          src="/assets/status-chain-cross.png"
          alt=""
        />
      )}
      {statuses.includes("selfDestruction") && (
        <img
          className="status-whole status-self-destruction"
          src="/assets/status-crack-edge.png"
          alt=""
        />
      )}
    </span>
  );
}
