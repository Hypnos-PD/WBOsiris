import { useState } from "react";
import { ImageOff } from "lucide-react";
import "./card-art.css";

export function CardArt({ src, alt = "", loading }: { src?: string; alt?: string; loading?: "lazy" | "eager" }) {
  const [failed, setFailed] = useState<string | null>(null);
  if (!src || failed === src) return <span className="card-art-missing" role="img" aria-label={alt ? `${alt}：暂无卡图` : "暂无卡图"}><ImageOff size={24} aria-hidden="true"/><small>暂无卡图</small></span>;
  return <img src={src} alt={alt} loading={loading} onError={() => setFailed(src)} />;
}
