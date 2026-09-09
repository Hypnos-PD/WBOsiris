import { useRef, useState } from "react";
import { invitationURL } from "./matchStorage";
import "./rooms.css";

export function RoomInvitation({ id, code }: { id: string; code?: string }) {
  const [status, setStatus] = useState("");
  const input = useRef<HTMLInputElement>(null);
  const link = code ? invitationURL(location.href, id, code) : "";
  return <div className="room-invitation">
    <small>{code ? `邀请码 ${code}` : "正在恢复邀请信息"}</small>
    {link && <><label>邀请链接<input ref={input} aria-label="邀请链接" readOnly value={link} onFocus={(event) => event.target.select()}/></label>
      <button onClick={async () => {
        try { await navigator.clipboard.writeText(link); setStatus("邀请链接已复制"); }
        catch { input.current?.focus(); input.current?.select(); setStatus("自动复制不可用，已选中链接，请手动复制"); }
      }}>复制邀请链接</button>
    </>}
    {status && <span role="status">{status}</span>}
  </div>;
}
