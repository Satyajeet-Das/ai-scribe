/**
 * Test doubles for the browser APIs the voice services use. They record what
 * was asked of them and let a test play the part of the server or the device.
 */
import { vi } from "vitest";
import { FRAME_HEADER_BYTES, PROTOCOL_VERSION, type ReadyPayload } from "../protocol";

// ---------------------------------------------------------------------------
// WebSocket
// ---------------------------------------------------------------------------

export class FakeWebSocket {
  static instances: FakeWebSocket[] = [];
  static reset(): void {
    FakeWebSocket.instances = [];
  }
  static get last(): FakeWebSocket {
    return FakeWebSocket.instances[FakeWebSocket.instances.length - 1];
  }
  /** Sockets that have not been closed by either side. */
  static get live(): FakeWebSocket[] {
    return FakeWebSocket.instances.filter((s) => s.readyState !== 3);
  }

  readyState = 0;
  binaryType = "blob";
  bufferedAmount = 0;
  sent: Array<string | ArrayBuffer> = [];
  closedWith: { code?: number; reason?: string } | null = null;

  onopen: (() => void) | null = null;
  onmessage: ((event: { data: unknown }) => void) | null = null;
  onclose: ((event: { code: number }) => void) | null = null;
  onerror: (() => void) | null = null;

  constructor(
    public readonly url: string,
    public readonly protocols: string[]
  ) {
    FakeWebSocket.instances.push(this);
  }

  send(data: string | ArrayBuffer): void {
    if (this.readyState !== 1) throw new Error("socket is not open");
    this.sent.push(data);
  }

  close(code?: number, reason?: string): void {
    this.readyState = 3;
    this.closedWith = { code, reason };
  }

  // --- the server's side ---------------------------------------------------

  accept(): void {
    this.readyState = 1;
    this.onopen?.();
  }

  serverSend(type: string, payload?: unknown, replyTo?: string): void {
    this.onmessage?.({
      data: JSON.stringify({ v: PROTOCOL_VERSION, type, replyTo, ts: 1, payload }),
    });
  }

  serverRaw(data: unknown): void {
    this.onmessage?.({ data });
  }

  serverAudio(id: string, seq: number, samples: number): void {
    const buffer = new ArrayBuffer(FRAME_HEADER_BYTES + samples * 2);
    const bytes = new Uint8Array(buffer);
    bytes[0] = 1;
    const hex = id.replace(/-/g, "");
    for (let i = 0; i < 16; i++) bytes[1 + i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16);
    new DataView(buffer).setUint32(17, seq, false);
    this.onmessage?.({ data: buffer });
  }

  serverClose(code: number): void {
    this.readyState = 3;
    this.onclose?.({ code });
  }

  /** Accepts the connection and completes the handshake. */
  handshake(overrides: Partial<ReadyPayload> = {}): void {
    this.accept();
    this.serverSend("ready", readyPayload(overrides));
  }

  // --- what the client sent ------------------------------------------------

  messages(): Array<{ v: number; type: string; id: string; payload?: Record<string, unknown> }> {
    return this.sent.filter((d): d is string => typeof d === "string").map((d) => JSON.parse(d));
  }

  ofType(type: string) {
    return this.messages().filter((m) => m.type === type);
  }

  binary(): ArrayBuffer[] {
    return this.sent.filter((d): d is ArrayBuffer => typeof d !== "string");
  }
}

export const QUESTION_1 = {
  id: "11111111-1111-4111-8111-111111111111",
  examId: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee",
  questionNumber: 1,
  text: "What is the capital of France?",
  type: "MCQ" as const,
  points: 1,
  options: [
    {
      id: "aaaaaaaa-0000-4000-8000-000000000001",
      optionKey: "A",
      optionText: "London",
      displayOrder: 0,
    },
    {
      id: "aaaaaaaa-0000-4000-8000-000000000002",
      optionKey: "B",
      optionText: "Paris",
      displayOrder: 1,
    },
  ],
  isFirst: true,
  isLast: false,
  totalQuestions: 2,
  currentIndex: 1,
};

export const QUESTION_2 = {
  ...QUESTION_1,
  id: "22222222-2222-4222-8222-222222222222",
  questionNumber: 2,
  text: "Which planet is red?",
  isFirst: false,
  isLast: true,
  currentIndex: 2,
};

export function readyPayload(overrides: Partial<ReadyPayload> = {}): ReadyPayload {
  return {
    connectionId: "conn-1",
    protocol: 1,
    sessionId: "session-1",
    resumed: false,
    limits: {
      maxMessageBytes: 65536,
      maxChunkBytes: 32768,
      maxStreamBytes: 2 * 1024 * 1024,
      maxStreamMillis: 45000,
      audioEncodings: ["pcm_s16le"],
      audioSampleRates: [8000, 16000, 24000, 48000],
      audioChannels: 1,
      confirmTtlMillis: 45000,
      binaryHeaderBytes: 21,
    },
    capabilities: {
      stt: true,
      tts: true,
      ttsFormat: { encoding: "pcm_s16le", sampleRate: 24000, channels: 1 },
      intentClassifier: "rules",
    },
    session: { status: "IN_PROGRESS", remainingSeconds: 3000, question: QUESTION_1 },
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Web Audio and getUserMedia
// ---------------------------------------------------------------------------

export class FakeNode {
  connected: unknown[] = [];
  disconnected = false;
  onaudioprocess: ((event: unknown) => void) | null = null;
  gain = { value: 1 };
  connect(target: unknown): void {
    this.connected.push(target);
  }
  disconnect(): void {
    this.disconnected = true;
  }
}

export class FakeSource extends FakeNode {
  buffer: { duration: number; length: number } | null = null;
  startedAt: number | null = null;
  stopped = false;
  onended: (() => void) | null = null;
  start(at: number): void {
    this.startedAt = at;
  }
  stop(): void {
    this.stopped = true;
  }
  /** Simulates the source reaching the end of its buffer. */
  finish(): void {
    this.onended?.();
  }
}

export class FakeAudioContext {
  static instances: FakeAudioContext[] = [];
  static initialState: AudioContextState = "running";
  static resumable = true;
  static sampleRate = 48000;
  static reset(): void {
    FakeAudioContext.instances = [];
    FakeAudioContext.initialState = "running";
    FakeAudioContext.resumable = true;
    FakeAudioContext.sampleRate = 48000;
  }

  state: AudioContextState = FakeAudioContext.initialState;
  sampleRate = FakeAudioContext.sampleRate;
  currentTime = 0;
  destination = new FakeNode();
  sources: FakeSource[] = [];
  nodes: FakeNode[] = [];
  processor: FakeNode | null = null;

  constructor() {
    FakeAudioContext.instances.push(this);
  }

  resume = vi.fn(async () => {
    if (FakeAudioContext.resumable) this.state = "running";
  });

  close = vi.fn(async () => {
    this.state = "closed";
  });

  createBuffer(_channels: number, length: number, sampleRate: number) {
    const data = new Float32Array(length);
    return { length, sampleRate, duration: length / sampleRate, getChannelData: () => data };
  }

  createBufferSource(): FakeSource {
    const source = new FakeSource();
    this.sources.push(source);
    return source;
  }

  createMediaStreamSource(): FakeNode {
    const node = new FakeNode();
    this.nodes.push(node);
    return node;
  }

  createGain(): FakeNode {
    const node = new FakeNode();
    this.nodes.push(node);
    return node;
  }

  createScriptProcessor(): FakeNode {
    const node = new FakeNode();
    this.nodes.push(node);
    this.processor = node;
    return node;
  }

  /** Feeds microphone samples through the capture graph. */
  feed(samples: Float32Array): void {
    this.processor?.onaudioprocess?.({ inputBuffer: { getChannelData: () => samples } });
  }
}

export class FakeTrack {
  stopped = false;
  private listeners = new Map<string, Set<() => void>>();
  stop(): void {
    this.stopped = true;
  }
  addEventListener(type: string, fn: () => void): void {
    if (!this.listeners.has(type)) this.listeners.set(type, new Set());
    this.listeners.get(type)!.add(fn);
  }
  removeEventListener(type: string, fn: () => void): void {
    this.listeners.get(type)?.delete(fn);
  }
  listenerCount(type: string): number {
    return this.listeners.get(type)?.size ?? 0;
  }
  /** Simulates the device being unplugged or the permission revoked. */
  end(): void {
    this.listeners.get("ended")?.forEach((fn) => fn());
  }
}

export class FakeMediaStream {
  tracks = [new FakeTrack()];
  getTracks(): FakeTrack[] {
    return this.tracks;
  }
  getAudioTracks(): FakeTrack[] {
    return this.tracks;
  }
}

export function domError(name: string): Error {
  const err = new Error(name);
  err.name = name;
  return err;
}
