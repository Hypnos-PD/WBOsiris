import type { Remote } from "./gameTypes.ts";

export type MatchAuth = { id: string; token: string; side: string };
export type ConnectionStatus = "connecting" | "connected" | "sending" | "reconnecting";
type Options = {
  base: string;
  auth: MatchAuth;
  onRemote: (remote: Remote) => void;
  onStatus: (status: ConnectionStatus) => void;
  onError: (message: string) => void;
  fetch?: typeof fetch;
  interval?: number;
  timeout?: number;
};

function decodeRemote(value: unknown, auth: MatchAuth): Remote {
  const data = value as Remote;
  const state = data?.state;
  if (data?.matchId !== auth.id || data.side !== auth.side || state?.viewer !== auth.side ||
      !Number.isSafeInteger(state?.revision) || state!.revision! < 0 ||
      !["own", "oppo"].includes(state?.turn?.active) || !Number.isSafeInteger(state?.turn?.number) ||
      !Array.isArray(data.legalActions) || data.events != null && !Array.isArray(data.events)) {
    throw new Error("对局响应无效");
  }
  for (const player of [state.own, state.oppo]) {
    if (!player || !Number.isFinite(player.pp) || !Number.isFinite(player.maxpp) ||
        !Number.isFinite(player.leaderLife) || !Array.isArray(player.field) ||
        player.hand !== undefined && !Array.isArray(player.hand)) throw new Error("对局响应无效");
  }
  return { ...data, playerToken: auth.token, events: data.events ?? [] };
}

export class MatchConnection {
  private options: Options;
  private stopped = false;
  private busy = false;
  private synchronized = false;
  private epoch = 0;
  private revision = -1;
  private timer: ReturnType<typeof setTimeout> | undefined;
  private request: AbortController | undefined;

  constructor(options: Options) { this.options = options; }

  start() {
    this.options.onStatus("connecting");
    void this.refresh();
  }

  stop() {
    this.stopped = true;
    this.epoch++;
    clearTimeout(this.timer);
    this.request?.abort();
  }

  private async read(path: string, body?: Record<string, unknown>) {
    const controller = new AbortController();
    this.request = controller;
    const timeout = setTimeout(() => controller.abort(), this.options.timeout ?? 10000);
    try {
      const response = await (this.options.fetch ?? fetch)(`${this.options.base}/api/matches/${this.options.auth.id}${path}`, {
        method: body === undefined ? "GET" : "POST",
        cache: "no-store",
        signal: controller.signal,
        headers: { Authorization: `Bearer ${this.options.auth.token}`, ...(body === undefined ? {} : { "Content-Type": "application/json" }) },
        ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      });
      if (!response.ok) throw new Error(`对局请求失败 (${response.status})`);
      let value: unknown;
      try { value = await response.json(); }
      catch { throw new Error("对局响应无效"); }
      return decodeRemote(value, this.options.auth);
    } finally {
      clearTimeout(timeout);
      if (this.request === controller) this.request = undefined;
    }
  }

  private accept(data: Remote) {
    if (data.state.revision! < this.revision) return false;
    this.revision = data.state.revision!;
    this.options.onRemote(data);
    return true;
  }

  private schedule() {
    if (!this.stopped) this.timer = setTimeout(() => void this.refresh(), this.options.interval ?? 700);
  }

  private async refresh() {
    if (this.stopped || this.busy) return;
    const epoch = ++this.epoch;
    try {
      const data = await this.read("");
      if (this.stopped || epoch !== this.epoch) return;
      if (this.accept(data)) {
        this.synchronized = true;
        this.options.onStatus("connected");
      }
    } catch {
      if (this.stopped || epoch !== this.epoch) return;
      this.synchronized = false;
      this.options.onStatus("reconnecting");
    } finally {
      if (epoch === this.epoch) this.schedule();
    }
  }

  async submit(body: Record<string, unknown>, path = ""): Promise<boolean> {
    if (this.stopped || this.busy || !this.synchronized) return false;
    this.busy = true;
    this.synchronized = false;
    const epoch = ++this.epoch;
    clearTimeout(this.timer);
    this.request?.abort();
    this.options.onStatus("sending");
    try {
      const data = await this.read(path, { ...body, expectedRevision: this.revision });
      if (this.stopped || epoch !== this.epoch) return false;
      if (!this.accept(data)) throw new Error("对局状态已更新");
      this.synchronized = true;
      this.options.onStatus("connected");
      const result = data.result;
      if (result && !["completed", "suspended"].includes(result.status)) {
        this.options.onError(result.errorCode === "stale_state" ? "局面已更新，操作未执行" : `操作未完成：${result.illegalCode || result.errorCode || result.status}`);
        return false;
      }
      return true;
    } catch (error) {
      if (this.stopped || epoch !== this.epoch) return false;
      this.options.onError(error instanceof Error && error.name !== "AbortError" && error.name !== "TypeError" ? error.message : "未收到操作结果，正在同步对局");
      this.options.onStatus("reconnecting");
      return false;
    } finally {
      this.busy = false;
      if (!this.stopped && epoch === this.epoch) {
        if (this.synchronized) this.schedule();
        else void this.refresh();
      }
    }
  }
}
