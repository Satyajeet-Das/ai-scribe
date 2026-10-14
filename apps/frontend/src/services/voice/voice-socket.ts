/**
 * Typed, reconnecting client for the exam voice WebSocket.
 *
 * It owns exactly one socket at a time, validates every incoming message, and
 * reconnects with bounded exponential backoff and jitter. It knows nothing
 * about React, audio devices or exam rules.
 */
import {
  AUTH_SUBPROTOCOL_PREFIX,
  CLOSE_CODES,
  SUBPROTOCOL,
  decodeAudioFrame,
  encodeAudioFrame,
  encodeClientMessage,
  parseServerMessage,
  type AudioFrame,
  type ClientMessageType,
  type ClientPayloads,
  type ParseFailure,
  type ServerMessage,
} from "./protocol";

export type ConnectionStatus =
  | "idle"
  | "connecting"
  | "connected"
  | "reconnecting"
  | "disconnected"
  | "error";

/** Why the client stopped for good (status "error"). None of these is retried automatically. */
export type FatalReason = "auth" | "forbidden" | "session_ended" | "replaced" | "protocol";

export interface StatusDetail {
  /** 1-based reconnect attempt, when status is "reconnecting". */
  attempt?: number;
  /** Set when status is "error". */
  fatal?: FatalReason;
  /** The server's reason from `session.closed`, when it gave one. */
  sessionReason?: string;
}

/**
 * The upgrade handshake fails with no detail visible to a browser (close code
 * 1006 whatever the HTTP status was). The probe asks the HTTP API, which is
 * the authority on the session, why.
 */
export type ProbeResult = "ok" | "ended" | "unauthorized" | "forbidden" | "unreachable";

export interface VoiceSocketHandlers {
  onStatus(status: ConnectionStatus, detail: StatusDetail): void;
  onMessage(message: ServerMessage): void;
  onAudio(frame: AudioFrame): void;
  /** Called for every incoming frame that fails validation. */
  onInvalid?(reason: ParseFailure | "bad_frame"): void;
}

export interface VoiceSocketOptions {
  url: string;
  /** Returns the current access token. It is sent as a subprotocol, never in the URL. */
  getToken(): string | null;
  /** Obtains a fresh access token, or null if the login has ended. */
  refreshToken(): Promise<string | null>;
  probe(): Promise<ProbeResult>;
  handlers: VoiceSocketHandlers;

  maxAttempts?: number;
  baseDelayMs?: number;
  maxDelayMs?: number;
  handshakeTimeoutMs?: number;
  pingIntervalMs?: number;
  pongTimeoutMs?: number;
  /** Consecutive invalid messages tolerated before the connection is dropped. */
  maxInvalidMessages?: number;
  random?: () => number;
  WebSocketCtor?: typeof WebSocket;
}

const WS_OPEN = 1;

export class VoiceSocket {
  private readonly opts: Required<Omit<VoiceSocketOptions, "WebSocketCtor">> & {
    WebSocketCtor?: typeof WebSocket;
  };

  private ws: WebSocket | null = null;
  /** Bumped for every socket and on stop, so callbacks from an old socket are ignored. */
  private generation = 0;
  private status: ConnectionStatus = "idle";
  private attempt = 0;
  private ready = false;
  private disposed = false;
  private sessionReason: string | undefined;
  private invalidStreak = 0;
  private nextId = 0;

  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private handshakeTimer: ReturnType<typeof setTimeout> | null = null;
  private pingTimer: ReturnType<typeof setInterval> | null = null;
  private pongTimer: ReturnType<typeof setTimeout> | null = null;

  constructor(options: VoiceSocketOptions) {
    this.opts = {
      maxAttempts: 8,
      baseDelayMs: 500,
      maxDelayMs: 15_000,
      handshakeTimeoutMs: 10_000,
      pingIntervalMs: 25_000,
      pongTimeoutMs: 10_000,
      maxInvalidMessages: 20,
      random: Math.random,
      ...options,
    };
  }

  getStatus(): ConnectionStatus {
    return this.status;
  }

  isConnected(): boolean {
    return this.status === "connected";
  }

  /** Bytes queued in the browser but not yet sent; used to detect a slow network. */
  bufferedAmount(): number {
    return this.ws?.bufferedAmount ?? 0;
  }

  /** Opens the connection. A no-op while a connection or a retry is already in progress. */
  start(): void {
    if (this.disposed) return;
    if (this.ws || this.reconnectTimer) return;
    if (
      this.status === "connecting" ||
      this.status === "connected" ||
      this.status === "reconnecting"
    ) {
      return;
    }
    this.attempt = 0;
    this.sessionReason = undefined;
    if (typeof window !== "undefined") {
      window.addEventListener("online", this.handleOnline);
    }
    this.open("connecting");
  }

  /** Manual retry after automatic retries were exhausted or a fatal error. */
  retry(): void {
    if (this.disposed) return;
    this.stop();
    this.start();
  }

  /** Closes the connection and cancels every timer. The client can be started again. */
  stop(): void {
    this.generation++;
    this.clearTimers();
    if (typeof window !== "undefined") {
      window.removeEventListener("online", this.handleOnline);
    }
    const ws = this.ws;
    this.ws = null;
    this.ready = false;
    if (ws) {
      this.detach(ws);
      try {
        ws.close(CLOSE_CODES.normal, "client_stop");
      } catch {
        // Closing a socket that never opened can throw; there is nothing to undo.
      }
    }
    this.setStatus("idle", {});
  }

  /** Stops permanently. */
  dispose(): void {
    if (this.disposed) return;
    this.stop();
    this.disposed = true;
  }

  /**
   * Sends a JSON message and returns its request ID, or null when the
   * connection is not ready. Nothing is queued for later: a message that
   * cannot be sent now would be stale by the time it could.
   */
  send<T extends ClientMessageType>(type: T, payload: ClientPayloads[T]): string | null {
    const ws = this.ws;
    if (!ws || !this.ready || ws.readyState !== WS_OPEN) return null;
    const id = `c${++this.nextId}-${Math.floor(this.opts.random() * 0xffffff).toString(36)}`;
    try {
      ws.send(encodeClientMessage(id, type, payload));
    } catch {
      return null;
    }
    return id;
  }

  /** Sends one audio chunk. Returns false when the connection is not ready. */
  sendAudio(streamId: string, seq: number, pcm: Int16Array): boolean {
    const ws = this.ws;
    if (!ws || !this.ready || ws.readyState !== WS_OPEN) return false;
    try {
      ws.send(encodeAudioFrame(streamId, seq, pcm));
    } catch {
      return false;
    }
    return true;
  }

  // -------------------------------------------------------------------------

  private setStatus(status: ConnectionStatus, detail: StatusDetail): void {
    if (this.status === status && status !== "reconnecting") return;
    this.status = status;
    this.opts.handlers.onStatus(status, detail);
  }

  private fatal(reason: FatalReason): void {
    this.clearTimers();
    if (typeof window !== "undefined") {
      window.removeEventListener("online", this.handleOnline);
    }
    this.setStatus("error", { fatal: reason, sessionReason: this.sessionReason });
  }

  private clearTimers(): void {
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
    if (this.handshakeTimer) clearTimeout(this.handshakeTimer);
    if (this.pingTimer) clearInterval(this.pingTimer);
    if (this.pongTimer) clearTimeout(this.pongTimer);
    this.reconnectTimer = this.handshakeTimer = this.pongTimer = null;
    this.pingTimer = null;
  }

  private detach(ws: WebSocket): void {
    ws.onopen = ws.onmessage = ws.onclose = ws.onerror = null;
  }

  private open(status: "connecting" | "reconnecting"): void {
    const token = this.opts.getToken();
    if (!token) {
      this.fatal("auth");
      return;
    }
    const Ctor = this.opts.WebSocketCtor ?? globalThis.WebSocket;
    if (typeof Ctor !== "function") {
      this.fatal("protocol");
      return;
    }

    const generation = ++this.generation;
    this.ready = false;
    this.invalidStreak = 0;
    if (status === "connecting") this.setStatus("connecting", {});

    let ws: WebSocket;
    try {
      ws = new Ctor(this.opts.url, [SUBPROTOCOL, AUTH_SUBPROTOCOL_PREFIX + token]);
    } catch {
      this.scheduleReconnect();
      return;
    }
    ws.binaryType = "arraybuffer";
    this.ws = ws;

    // The connection only counts once the server's `ready` message arrives.
    this.handshakeTimer = setTimeout(() => {
      if (generation !== this.generation) return;
      this.drop(ws);
    }, this.opts.handshakeTimeoutMs);

    ws.onmessage = (event: MessageEvent) => {
      if (generation !== this.generation) return;
      this.handleMessage(event.data);
    };
    ws.onclose = (event: CloseEvent) => {
      if (generation !== this.generation) return;
      void this.handleClose(event.code);
    };
    // An error is always followed by a close event, which does the handling.
    ws.onerror = () => {};
  }

  /** Abandons a socket that is not behaving and lets the close path decide what next. */
  private drop(ws: WebSocket): void {
    this.detach(ws);
    try {
      ws.close();
    } catch {
      // ignored
    }
    if (this.ws === ws) void this.handleClose(1006);
  }

  private handleMessage(data: unknown): void {
    if (typeof data === "string") {
      const parsed = parseServerMessage(data);
      if (!parsed.ok) {
        this.invalid(parsed.reason);
        return;
      }
      this.invalidStreak = 0;
      this.dispatch(parsed.message);
      return;
    }
    if (data instanceof ArrayBuffer) {
      const frame = decodeAudioFrame(data);
      if (!frame) {
        this.invalid("bad_frame");
        return;
      }
      if (this.ready) this.opts.handlers.onAudio(frame);
      return;
    }
    this.invalid("bad_frame");
  }

  private invalid(reason: ParseFailure | "bad_frame"): void {
    this.opts.handlers.onInvalid?.(reason);
    if (++this.invalidStreak >= this.opts.maxInvalidMessages && this.ws) {
      // Whatever is on the other end is not speaking this protocol.
      this.drop(this.ws);
    }
  }

  private dispatch(message: ServerMessage): void {
    switch (message.type) {
      case "ready":
        if (this.handshakeTimer) clearTimeout(this.handshakeTimer);
        this.handshakeTimer = null;
        this.ready = true;
        this.attempt = 0;
        this.sessionReason = undefined;
        this.startHeartbeat();
        this.setStatus("connected", {});
        break;
      case "pong":
        if (this.pongTimer) clearTimeout(this.pongTimer);
        this.pongTimer = null;
        return;
      case "session.closed":
        this.sessionReason = message.payload.reason;
        break;
      case "auth.expiring":
        void this.renewToken();
        break;
      default:
        break;
    }
    // Nothing is delivered before the handshake completes.
    if (this.ready) this.opts.handlers.onMessage(message);
  }

  private startHeartbeat(): void {
    if (this.pingTimer) clearInterval(this.pingTimer);
    this.pingTimer = setInterval(() => {
      if (this.pongTimer) return; // still waiting for the previous pong
      if (this.send("ping", undefined) === null) return;
      const ws = this.ws;
      this.pongTimer = setTimeout(() => {
        this.pongTimer = null;
        if (ws && this.ws === ws) this.drop(ws);
      }, this.opts.pongTimeoutMs);
    }, this.opts.pingIntervalMs);
  }

  private async renewToken(): Promise<void> {
    const generation = this.generation;
    let token: string | null = null;
    try {
      token = await this.opts.refreshToken();
    } catch {
      token = null;
    }
    // If the refresh failed the server closes with 4401 and handleClose reports it.
    if (token && generation === this.generation) {
      this.send("auth.refresh", { token });
    }
  }

  private async handleClose(code: number): Promise<void> {
    const wasReady = this.ready;
    const generation = ++this.generation;
    this.ws = null;
    this.ready = false;
    if (this.handshakeTimer) clearTimeout(this.handshakeTimer);
    if (this.pingTimer) clearInterval(this.pingTimer);
    if (this.pongTimer) clearTimeout(this.pongTimer);
    this.handshakeTimer = this.pongTimer = null;
    this.pingTimer = null;

    switch (code) {
      case CLOSE_CODES.sessionEnded:
        return this.fatal("session_ended");
      case CLOSE_CODES.forbidden:
        return this.fatal("forbidden");
      case CLOSE_CODES.replaced:
        // Another tab or device took over. Reconnecting would just evict it
        // in turn, so the student decides.
        return this.fatal("replaced");
      case CLOSE_CODES.policyViolation:
        return this.fatal("protocol");
      case CLOSE_CODES.unauthorized: {
        this.setStatus("reconnecting", { attempt: this.attempt + 1 });
        let token: string | null = null;
        try {
          token = await this.opts.refreshToken();
        } catch {
          token = null;
        }
        if (generation !== this.generation) return;
        if (!token) return this.fatal("auth");
        return this.scheduleReconnect();
      }
      default:
        break;
    }

    if (!wasReady) {
      // The upgrade itself was refused or never completed. Ask the HTTP API why.
      this.setStatus("reconnecting", { attempt: this.attempt + 1 });
      let result: ProbeResult = "unreachable";
      try {
        result = await this.opts.probe();
      } catch {
        result = "unreachable";
      }
      if (generation !== this.generation) return;
      if (result === "ended") return this.fatal("session_ended");
      if (result === "unauthorized") return this.fatal("auth");
      if (result === "forbidden") return this.fatal("forbidden");
    }
    this.scheduleReconnect();
  }

  private scheduleReconnect(): void {
    if (this.reconnectTimer) return; // never more than one retry loop
    this.attempt++;
    if (this.attempt > this.opts.maxAttempts) {
      this.clearTimers();
      this.setStatus("disconnected", {});
      return;
    }
    const ceiling = Math.min(this.opts.maxDelayMs, this.opts.baseDelayMs * 2 ** (this.attempt - 1));
    // Full jitter, with a floor so a burst of clients never retries at once at zero delay.
    const delay = Math.max(100, Math.floor(this.opts.random() * ceiling));
    this.setStatus("reconnecting", { attempt: this.attempt });

    const generation = this.generation;
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      if (generation !== this.generation) return;
      this.open("reconnecting");
    }, delay);
  }

  private handleOnline = (): void => {
    // The network came back: retry now instead of waiting out the backoff.
    if (!this.reconnectTimer) return;
    clearTimeout(this.reconnectTimer);
    this.reconnectTimer = null;
    this.open("reconnecting");
  };
}

/** Builds the WebSocket URL from the HTTP API base. The URL never carries credentials. */
export function voiceSocketUrl(apiBaseUrl: string, sessionId: string, origin?: string): string {
  const base =
    origin ?? (typeof window !== "undefined" ? window.location.origin : "http://localhost");
  const url = new URL(apiBaseUrl, base);
  url.pathname =
    url.pathname.replace(/\/+$/, "") + `/sessions/${encodeURIComponent(sessionId)}/voice`;
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  url.username = "";
  url.password = "";
  url.search = "";
  url.hash = "";
  return url.toString();
}
