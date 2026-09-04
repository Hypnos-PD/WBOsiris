import { createRoot } from "react-dom/client";
import * as React from "react";
import prefabData from "./statusPrefabData.json";
import { StatusEffects, type StatusEffect } from "./StatusEffects";
import "./status-preview.css";

type AssetRecord = { name: string; kind: "Texture2D" | "Mesh" };
type MaterialRecord = {
  name: string;
  shader: number;
  keywords: string[];
  textures: Record<
    string,
    { asset: number; scale: number[]; offset: number[] }
  >;
};
type PrefabRecord = {
  assets: Record<string, string>;
  assetKinds: Record<string, "Texture2D" | "Mesh">;
  materials: Record<string, MaterialRecord>;
};

const records = prefabData as Record<string, PrefabRecord>;
function assetRecords(prefab: PrefabRecord): AssetRecord[] {
  return Object.entries(prefab.assets)
    .filter(([id]) => Boolean(prefab.assetKinds[id]))
    .map(([id, name]) => ({ name, kind: prefab.assetKinds[id] ?? "Mesh" }));
}

function StaticAsset({
  prefab,
  asset,
}: {
  prefab: string;
  asset: AssetRecord;
}) {
  const [hidden, setHidden] = React.useState(false);
  if (hidden || asset.kind === "Mesh")
    return asset.kind === "Mesh" ? (
      <li className="mesh-asset">
        <code>{asset.name}.obj</code>
      </li>
    ) : null;
  return (
    <li>
      <img
        src={`/assets/status-prefabs/${prefab}/${asset.name}.png`}
        alt={asset.name}
        onError={() => setHidden(true)}
      />
      <code>{asset.name}.png</code>
    </li>
  );
}

function PrefabCard({ prefab }: { prefab: string }) {
  const data = records[prefab];
  const materials = Object.values(data.materials);
  const assets = assetRecords(data);
  return (
    <article>
      <header>
        <h2>{prefab}</h2>
        <p>{materials.map((material) => material.name).join(" / ")}</p>
      </header>
      <ul>
        {assets.map((asset) => (
          <StaticAsset
            key={`${asset.kind}:${asset.name}`}
            prefab={prefab}
            asset={asset}
          />
        ))}
      </ul>
      <details>
        <summary>材质参数</summary>
        {materials.map((material) => (
          <section key={material.name}>
            <strong>{material.name}</strong>
            <code>{material.keywords.join(" ") || "无关键词"}</code>
            <div>
              {Object.entries(material.textures)
                .filter(([, texture]) => texture.asset !== 0)
                .map(([slot, texture]) => (
                  <span key={slot}>
                    {slot}:{" "}
                    {data.assets[String(texture.asset)] ?? texture.asset} (
                    {texture.scale.join("x")})
                  </span>
                ))}
            </div>
          </section>
        ))}
      </details>
    </article>
  );
}

function Composite({ status }: { status: StatusEffect }) {
  return <div className="composite"><img src="/assets/card-10002110.webp" alt="" /><StatusEffects statuses={[status]} /></div>;
}

function CompositeGuide() {
  return (
    <section className="composite-guide">
      <h2>拼接确认区</h2>
      <p>仅按已确认规则做静态组合，不使用 Prefab 推断。</p>
      <div className="composite-row">
        <figure>
          <Composite status="barrier" />
          <figcaption>屏障：蓝色圆形膜</figcaption>
        </figure>
        <figure>
          <Composite status="abilityProtected" />
          <figcaption>能力破坏抗性：橙黄色圆形膜</figcaption>
        </figure>
        <figure>
          <Composite status="damageReduction" />
          <figcaption>限制伤害：圆膜 + 六边形网纹</figcaption>
        </figure>
        <figure>
          <Composite status="intimidate" />
          <figcaption>威慑：绿色毛糙菱形激波边框</figcaption>
        </figure>
        <figure>
          <Composite status="stealth" />
          <figcaption>潜行：覆盖卡面的连续雾气</figcaption>
        </figure>
        <figure>
          <Composite status="aura" />
          <figcaption>灵气：四向中心对称</figcaption>
        </figure>
        <figure>
          <Composite status="hold" />
          <figcaption>无法攻击随从或主战者</figcaption>
        </figure>
        <figure>
          <Composite status="selfDestruction" />
          <figcaption>对手回合结束时破坏本卡</figcaption>
        </figure>
      </div>
    </section>
  );
}

function Preview() {
  return (
    <main>
      <header className="page-header">
        <h1>状态 Prefab 静态素材</h1>
        <p>
          顶部是已确认拼法；下方是未处理的 Unity 原始纹理、Mesh、材质槽位和
          Shader 关键词。
        </p>
        <a href="/">返回牌桌</a>
      </header>
      <CompositeGuide />
      <section className="static-grid">
        {Object.keys(records)
          .sort()
          .map((prefab) => (
            <PrefabCard key={prefab} prefab={prefab} />
          ))}
      </section>
    </main>
  );
}

createRoot(document.getElementById("root")!).render(<Preview />);
