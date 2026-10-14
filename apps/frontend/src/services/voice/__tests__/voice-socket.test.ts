import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  VoiceSocket,
  type ProbeResult,
  type StatusDetail,
  type ConnectionStatus,
} from "../voice-socket";
import type { AudioFrame, ServerMessage } from "../protocol";
import { FakeWebSocket } from "./fakes";

const URL = "ws://localhost:8080/api/v1/sessions/s1/voice";
const RESPONSE_ID = "99999999-9999-4999-8999-999999999999";

interface Harness {
  socket: VoiceSocket;
  statuses: Array<{ status: ConnectionStatus; detail: StatusDetail }>;
  messages: ServerMessage[];
  audio: AudioFrame[];
  invalid: string[];
  token: { value: string | null };
  refreshToken: ReturnType<typeof vi.fn>;
  probe: ReturnType<typeof vi.fn>;
}

const created: VoiceSocket[] = [];

function setup(overrides: Partial<ConstructorParameters<typeof VoiceSocket>[0]> = {}): Harness {
  const h = {
    statuses: [] as Harness["statuses"],
    messages: [] as ServerMessage[],
    audio: [] as AudioFrame[],
    invalid: [] as string[],
    token: { value: "access-token" as string | null },
    refreshToken: vi.fn(async () => "fresh-token" as string | null),
    probe: vi.fn(async () => "ok" as ProbeResult),
  };
  const socket = new VoiceSocket({
    url: URL,
    getToken: () => h.token.value,
    refreshToken: h.refreshToken,
    probe: h.probe,
    handlers: {
      onStatus: (status, detail) => h.statuses.push({ status, detail }),
      onMessage: (m) => h.messages.push(m),
      onAudio: (f) => h.audio.push(f),
      onInvalid: (r) => h.invalid.push(r),
    },
    WebSocketCtor: FakeWebSocket as unknown as typeof WebSocket,
    random: () => 0.5,
    ...overrides,
  });
  created.push(socket);
  return { ...h, socket };
}

const lastStatus = (h: Harness) => h.statuses[h.statuses.length - 1];
/** Lets pending promise continuations (probe, token refresh) run. */
const flush = () => vi.advanceTimersByTimeAsync(0);

describe("VoiceSocket", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    FakeWebSocket.reset();
  });
  afterEach(() => {
    // Release window listeners and timers so tests cannot affect each other.
    created.splice(0).forEach((s) => s.dispose());
    vi.useRealTimers();
  });

  describe("connecting", () => {
    it("authenticates through the subprotocol, never the URL", () => {
      const h = setup();
      h.socket.start();

      const ws = FakeWebSocket.last;
      expect(ws.url).toBe(URL);
      expect(ws.url).not.toContain("access-token");
      expect(ws.protocols).toEqual(["scribe.v1", "scribe.auth.access-token"]);
      expect(ws.binaryType).toBe("arraybuffer");
      expect(lastStatus(h).status).toBe("connecting");
    });

    it("is connected only once the server's ready message arrives", () => {
      const h = setup();
      h.socket.start();
      FakeWebSocket.last.accept();
      expect(h.socket.isConnected()).toBe(false);
      expect(h.socket.send("ping", undefined)).toBeNull();

      FakeWebSocket.last.serverSend("ready", undefined);
      expect(h.socket.isConnected()).toBe(false);

      FakeWebSocket.last.handshake();
      expect(h.socket.isConnected()).toBe(true);
      expect(h.messages.map((m) => m.type)).toEqual(["ready"]);
    });

    it("never opens a second socket while one is live or a retry is pending", () => {
      const h = setup();
      h.socket.start();
      h.socket.start();
      FakeWebSocket.last.handshake();
      h.socket.start();
      expect(FakeWebSocket.instances).toHaveLength(1);

      FakeWebSocket.last.serverClose(1006);
      h.socket.start();
      h.socket.start();
      expect(FakeWebSocket.live).toHaveLength(0);
      vi.advanceTimersByTime(1000);
      expect(FakeWebSocket.live).toHaveLength(1);
      expect(FakeWebSocket.instances).toHaveLength(2);
    });

    it("fails with an auth error, without dialing, when there is no token", () => {
      const h = setup();
      h.token.value = null;
      h.socket.start();
      expect(FakeWebSocket.instances).toHaveLength(0);
      expect(lastStatus(h)).toEqual({
        status: "error",
        detail: { fatal: "auth", sessionReason: undefined },
      });
    });

    it("gives up on a handshake that never completes", async () => {
      const h = setup({ handshakeTimeoutMs: 1000 });
      h.socket.start();
      FakeWebSocket.last.accept();
      vi.advanceTimersByTime(1000);
      await flush();
      expect(FakeWebSocket.instances[0].closedWith).not.toBeNull();
      expect(h.probe).toHaveBeenCalledTimes(1);
      expect(lastStatus(h).status).toBe("reconnecting");
    });
  });

  describe("messages", () => {
    it("delivers validated messages and audio, and drops malformed ones", () => {
      const h = setup();
      h.socket.start();
      const ws = FakeWebSocket.last;
      ws.handshake();

      ws.serverSend("stt.final", { streamId: "s", text: "next" }, "c1");
      ws.serverRaw("{broken");
      ws.serverSend("exam.answers", { all: true });
      ws.serverSend("stt.final", { text: 42 });
      ws.serverRaw(new ArrayBuffer(3));
      ws.serverRaw(new Blob(["x"]));
      ws.serverAudio(RESPONSE_ID, 0, 160);

      expect(h.messages.map((m) => m.type)).toEqual(["ready", "stt.final"]);
      expect(h.invalid).toEqual([
        "not_json",
        "unknown_type",
        "bad_payload",
        "bad_frame",
        "bad_frame",
      ]);
      expect(h.audio).toHaveLength(1);
      expect(h.audio[0]).toMatchObject({ id: RESPONSE_ID, seq: 0 });
      expect(h.socket.isConnected()).toBe(true);
    });

    it("drops the connection after a run of invalid messages", async () => {
      const h = setup({ maxInvalidMessages: 3 });
      h.socket.start();
      const ws = FakeWebSocket.last;
      ws.handshake();
      ws.serverRaw("x");
      ws.serverRaw("y");
      expect(ws.closedWith).toBeNull();
      ws.serverRaw("z");
      await flush();
      expect(ws.closedWith).not.toBeNull();
      expect(lastStatus(h).status).toBe("reconnecting");
    });

    it("gives every request a unique id and sends nothing when not connected", () => {
      const h = setup();
      h.socket.start();
      const ws = FakeWebSocket.last;
      ws.handshake();

      const a = h.socket.send("command", { intent: "next_question" });
      const b = h.socket.send("command", { intent: "next_question" });
      expect(a).not.toBeNull();
      expect(a).not.toBe(b);
      expect(ws.ofType("command").map((m) => m.id)).toEqual([a, b]);
      expect(h.socket.sendAudio("11111111-1111-4111-8111-111111111111", 0, new Int16Array(4))).toBe(
        true
      );
      expect(ws.binary()).toHaveLength(1);

      ws.serverClose(1006);
      expect(h.socket.send("command", { intent: "next_question" })).toBeNull();
      expect(h.socket.sendAudio("11111111-1111-4111-8111-111111111111", 1, new Int16Array(4))).toBe(
        false
      );
    });

    it("ignores events from a socket it has already replaced", () => {
      const h = setup();
      h.socket.start();
      const old = FakeWebSocket.last;
      old.handshake();
      const handler = old.onmessage;
      h.socket.retry();
      FakeWebSocket.last.handshake();
      const before = h.messages.length;

      handler?.({
        data: JSON.stringify({ v: 1, type: "stt.final", payload: { streamId: "s", text: "late" } }),
      });
      expect(h.messages).toHaveLength(before);
    });
  });

  describe("heartbeat", () => {
    it("pings periodically and reconnects when the pong does not come", async () => {
      const h = setup({ pingIntervalMs: 1000, pongTimeoutMs: 500 });
      h.socket.start();
      const ws = FakeWebSocket.last;
      ws.handshake();

      vi.advanceTimersByTime(1000);
      const ping = ws.ofType("ping")[0];
      expect(ping).toBeDefined();
      ws.serverSend("pong", undefined, ping.id);
      vi.advanceTimersByTime(600);
      expect(h.socket.isConnected()).toBe(true);
      expect(h.messages.map((m) => m.type)).not.toContain("pong");

      vi.advanceTimersByTime(400);
      expect(ws.ofType("ping")).toHaveLength(2);
      vi.advanceTimersByTime(500);
      await flush();
      expect(ws.closedWith).not.toBeNull();
      expect(lastStatus(h).status).toBe("reconnecting");
    });
  });

  describe("reconnecting", () => {
    it("backs off exponentially with jitter, capped, and stops after the attempt limit", async () => {
      const h = setup({ maxAttempts: 4, baseDelayMs: 1000, maxDelayMs: 3000 });
      h.socket.start();
      FakeWebSocket.last.handshake();
      FakeWebSocket.last.serverClose(1006);

      // random() = 0.5 → half of min(cap, base * 2^(n-1)): 500, 1000, 1500 (capped), 1500.
      for (const [attempt, delay] of [
        [1, 500],
        [2, 1000],
        [3, 1500],
        [4, 1500],
      ]) {
        expect(lastStatus(h)).toEqual({ status: "reconnecting", detail: { attempt } });
        const before = FakeWebSocket.instances.length;
        vi.advanceTimersByTime(delay - 1);
        expect(FakeWebSocket.instances).toHaveLength(before);
        vi.advanceTimersByTime(1);
        expect(FakeWebSocket.instances).toHaveLength(before + 1);
        FakeWebSocket.last.serverClose(1006);
        await flush();
      }

      expect(lastStatus(h).status).toBe("disconnected");
      vi.advanceTimersByTime(600_000);
      expect(FakeWebSocket.instances).toHaveLength(5);
      expect(FakeWebSocket.live).toHaveLength(0);
    });

    it("resets the attempt count after a successful reconnect", async () => {
      const h = setup({ maxAttempts: 2, baseDelayMs: 1000 });
      h.socket.start();
      FakeWebSocket.last.handshake();
      for (let round = 0; round < 5; round++) {
        FakeWebSocket.last.serverClose(1001);
        await flush();
        expect(lastStatus(h)).toEqual({ status: "reconnecting", detail: { attempt: 1 } });
        vi.advanceTimersByTime(1000);
        FakeWebSocket.last.handshake();
        expect(h.socket.isConnected()).toBe(true);
      }
    });

    it("retries immediately when the browser comes back online", () => {
      const h = setup({ baseDelayMs: 60_000, maxDelayMs: 60_000 });
      h.socket.start();
      FakeWebSocket.last.handshake();
      FakeWebSocket.last.serverClose(1006);
      expect(FakeWebSocket.live).toHaveLength(0);

      window.dispatchEvent(new Event("online"));
      expect(FakeWebSocket.live).toHaveLength(1);
      vi.advanceTimersByTime(120_000);
      expect(FakeWebSocket.instances).toHaveLength(2);
    });

    it("lets the student retry manually after automatic retries are exhausted", async () => {
      const h = setup({ maxAttempts: 1, baseDelayMs: 100 });
      h.socket.start();
      FakeWebSocket.last.handshake();
      FakeWebSocket.last.serverClose(1006);
      vi.advanceTimersByTime(100);
      FakeWebSocket.last.serverClose(1006);
      await flush();
      expect(lastStatus(h).status).toBe("disconnected");

      h.socket.retry();
      FakeWebSocket.last.handshake();
      expect(h.socket.isConnected()).toBe(true);
    });
  });

  describe("server-initiated closes", () => {
    it.each([
      [4410, "session_ended"],
      [4403, "forbidden"],
      [4409, "replaced"],
      [1008, "protocol"],
    ] as const)("treats close code %i as final (%s) and does not reconnect", (code, fatal) => {
      const h = setup();
      h.socket.start();
      const ws = FakeWebSocket.last;
      ws.handshake();
      if (code === 4410) ws.serverSend("session.closed", { reason: "submitted" });
      ws.serverClose(code);

      expect(lastStatus(h).status).toBe("error");
      expect(lastStatus(h).detail.fatal).toBe(fatal);
      if (code === 4410) expect(lastStatus(h).detail.sessionReason).toBe("submitted");
      vi.advanceTimersByTime(600_000);
      expect(FakeWebSocket.instances).toHaveLength(1);
    });

    it("refreshes the token and reconnects after an auth-expiry close", async () => {
      const h = setup();
      h.refreshToken.mockImplementation(async () => {
        h.token.value = "fresh-token";
        return "fresh-token";
      });
      h.socket.start();
      FakeWebSocket.last.handshake();
      FakeWebSocket.last.serverClose(4401);
      await flush();
      vi.advanceTimersByTime(20_000);

      expect(h.refreshToken).toHaveBeenCalledTimes(1);
      expect(FakeWebSocket.last.protocols).toEqual(["scribe.v1", "scribe.auth.fresh-token"]);
    });

    it("stops with an auth error when the token cannot be refreshed", async () => {
      const h = setup();
      h.refreshToken.mockResolvedValue(null);
      h.socket.start();
      FakeWebSocket.last.handshake();
      FakeWebSocket.last.serverClose(4401);
      await flush();

      expect(lastStatus(h).detail.fatal).toBe("auth");
      vi.advanceTimersByTime(600_000);
      expect(FakeWebSocket.instances).toHaveLength(1);
    });

    it("sends auth.refresh with a new token when the server warns of expiry", async () => {
      const h = setup();
      h.socket.start();
      const ws = FakeWebSocket.last;
      ws.handshake();
      ws.serverSend("auth.expiring", { expiresAt: "2026-10-12T10:00:00Z" });
      await flush();

      expect(ws.ofType("auth.refresh")[0].payload).toEqual({ token: "fresh-token" });
    });
  });

  describe("refused upgrades", () => {
    it.each([
      ["ended", "session_ended"],
      ["unauthorized", "auth"],
      ["forbidden", "forbidden"],
    ] as const)("asks the HTTP API why, and stops when the answer is %s", async (probe, fatal) => {
      const h = setup();
      h.probe.mockResolvedValue(probe);
      h.socket.start();
      FakeWebSocket.last.serverClose(1006);
      await flush();

      expect(h.probe).toHaveBeenCalledTimes(1);
      expect(lastStatus(h).detail.fatal).toBe(fatal);
      vi.advanceTimersByTime(600_000);
      expect(FakeWebSocket.instances).toHaveLength(1);
    });

    it.each(["ok", "unreachable"] as const)(
      "retries with backoff when the probe says %s",
      async (probe) => {
        const h = setup();
        h.probe.mockResolvedValue(probe);
        h.socket.start();
        FakeWebSocket.last.serverClose(1006);
        await flush();

        expect(lastStatus(h).status).toBe("reconnecting");
        vi.advanceTimersByTime(20_000);
        expect(FakeWebSocket.instances).toHaveLength(2);
      }
    );
  });

  describe("stopping", () => {
    it("closes the socket, cancels timers and ignores late results", async () => {
      let resolveProbe: (r: ProbeResult) => void = () => {};
      const h = setup({ pingIntervalMs: 1000 });
      h.probe.mockImplementation(() => new Promise<ProbeResult>((r) => (resolveProbe = r)));
      h.socket.start();
      FakeWebSocket.last.serverClose(1006); // probe now in flight

      h.socket.stop();
      resolveProbe("ok");
      await flush();
      vi.advanceTimersByTime(600_000);

      expect(lastStatus(h).status).toBe("idle");
      expect(FakeWebSocket.instances).toHaveLength(1);
      expect(vi.getTimerCount()).toBe(0);
    });

    it("sends a normal close and removes its handlers", () => {
      const h = setup();
      h.socket.start();
      const ws = FakeWebSocket.last;
      ws.handshake();
      h.socket.stop();

      expect(ws.closedWith).toEqual({ code: 1000, reason: "client_stop" });
      expect(ws.onmessage).toBeNull();
      expect(ws.onclose).toBeNull();
      expect(vi.getTimerCount()).toBe(0);
    });

    it("cannot be restarted after dispose", () => {
      const h = setup();
      h.socket.start();
      h.socket.dispose();
      h.socket.start();
      h.socket.retry();
      window.dispatchEvent(new Event("online"));
      expect(FakeWebSocket.instances).toHaveLength(1);
    });
  });
});
