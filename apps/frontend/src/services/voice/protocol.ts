/**
 * Voice WebSocket protocol, version 1.
 *
 * This file mirrors the backend contract in apps/backend/internal/voice/protocol.go
 * (documented in docs/architecture/voice-protocol.md). Nothing here is invented:
 * every message type, payload field, close code and the binary frame layout
 * comes from that contract. Incoming messages are validated before use.
 */
import { z } from "zod";

export const PROTOCOL_VERSION = 1;
export const SUBPROTOCOL = "scribe.v1";
/** Browsers cannot set headers on a WebSocket, so the token rides in a subprotocol. */
export const AUTH_SUBPROTOCOL_PREFIX = "scribe.auth.";

/** The only audio encoding the backend accepts or emits. */
export const PCM_ENCODING = "pcm_s16le";

/** Application close codes sent by the backend. */
export const CLOSE_CODES = {
  normal: 1000,
  goingAway: 1001,
  policyViolation: 1008,
  messageTooBig: 1009,
  unauthorized: 4401,
  forbidden: 4403,
  replaced: 4409,
  sessionEnded: 4410,
  slowConsumer: 4429,
} as const;

/** Intents the backend accepts in a `command` message. */
export const COMMAND_INTENTS = [
  "repeat_question",
  "next_question",
  "previous_question",
  "select_option",
  "read_back_answer",
  "time_remaining",
  "confirm",
  "deny",
] as const;
export type CommandIntent = (typeof COMMAND_INTENTS)[number];

/**
 * What an utterance is for. It is chosen by the control the student used, never
 * inferred from what was said: "command" speech can never become answer text,
 * and "answer" speech is never interpreted as a command.
 */
export type AudioMode = "command" | "answer";

// ---------------------------------------------------------------------------
// Client → server payloads
// ---------------------------------------------------------------------------

export interface ClientPayloads {
  ping: undefined;
  "session.sync": undefined;
  "auth.refresh": { token: string };
  "audio.start": {
    streamId: string;
    encoding: string;
    sampleRate: number;
    channels: number;
    mode: AudioMode;
  };
  "audio.end": { streamId: string; chunks?: number };
  "audio.cancel": { streamId: string };
  "tts.cancel": { responseId?: string };
  command: {
    intent: CommandIntent;
    optionKey?: string;
    confirmationId?: string;
    /** The student's corrected wording; only with "confirm" of a dictated answer. */
    text?: string;
  };
}
export type ClientMessageType = keyof ClientPayloads;

// ---------------------------------------------------------------------------
// Server → client payloads (validated)
// ---------------------------------------------------------------------------

const audioFormatSchema = z.object({
  encoding: z.string(),
  sampleRate: z.number().int().positive(),
  channels: z.number().int().positive(),
});

const optionSchema = z.object({
  id: z.string(),
  optionKey: z.string(),
  optionText: z.string(),
  displayOrder: z.number(),
});

/** The student-facing question DTO. It has no notion of correctness. */
const questionSchema = z.object({
  id: z.string(),
  examId: z.string(),
  questionNumber: z.number(),
  text: z.string(),
  type: z.enum(["MCQ", "ESSAY", "VOICE"]),
  points: z.number(),
  options: z.array(optionSchema).optional(),
  isFirst: z.boolean(),
  isLast: z.boolean(),
  totalQuestions: z.number(),
  currentIndex: z.number(),
});

const sessionStateSchema = z.object({
  status: z.string(),
  remainingSeconds: z.number().optional(),
  question: questionSchema.optional(),
});

const limitsSchema = z.object({
  maxMessageBytes: z.number().positive(),
  maxChunkBytes: z.number().positive(),
  maxStreamBytes: z.number().positive(),
  maxStreamMillis: z.number().positive(),
  audioEncodings: z.array(z.string()),
  audioSampleRates: z.array(z.number()),
  audioChannels: z.number(),
  confirmTtlMillis: z.number(),
  binaryHeaderBytes: z.number(),
});

const serverSchemas = {
  ready: z.object({
    connectionId: z.string(),
    protocol: z.number(),
    sessionId: z.string(),
    resumed: z.boolean(),
    limits: limitsSchema,
    capabilities: z.object({
      stt: z.boolean(),
      tts: z.boolean(),
      /** True when dictated answers are kept as audio. Absent on older servers. */
      recording: z.boolean().optional(),
      ttsFormat: audioFormatSchema.optional(),
      intentClassifier: z.string(),
    }),
    session: sessionStateSchema,
  }),
  pong: z.undefined(),
  ack: z.undefined(),
  error: z.object({
    code: z.string(),
    message: z.string(),
    retryable: z.boolean(),
    streamId: z.string().optional(),
  }),
  "session.state": sessionStateSchema,
  "session.closed": z.object({ reason: z.string() }),
  "auth.expiring": z.object({ expiresAt: z.string() }),
  "stt.partial": z.object({ streamId: z.string(), text: z.string() }),
  "stt.final": z.object({ streamId: z.string(), text: z.string() }),
  intent: z.object({
    streamId: z.string().optional(),
    intent: z.string(),
    optionKey: z.string().optional(),
    confidence: z.number(),
    classifier: z.string(),
  }),
  "confirm.request": z.object({
    confirmationId: z.string(),
    action: z.string(),
    questionId: z.string(),
    optionKey: z.string().optional(),
    optionText: z.string().optional(),
    text: z.string().optional(),
    expiresAt: z.string(),
  }),
  "action.result": z.object({
    action: z.enum(["answer", "navigate"]),
    status: z.enum(["ok", "failed", "rejected", "cancelled"]),
    code: z.string().optional(),
    retryable: z.boolean().optional(),
    saved: z.boolean(),
    /** The student changed the recognized text before confirming. */
    corrected: z.boolean().optional(),
    duplicate: z.boolean().optional(),
    confirmationId: z.string().optional(),
    questionId: z.string().optional(),
    optionKey: z.string().optional(),
    answerId: z.string().optional(),
    savedAt: z.string().optional(),
    direction: z.string().optional(),
  }),
  response: z.object({
    responseId: z.string(),
    streamId: z.string().optional(),
    kind: z.string(),
    code: z.string().optional(),
    text: z.string(),
    audio: z.boolean(),
  }),
  /** Whether the audio of a dictated answer was kept. Says nothing about the answer. */
  "recording.status": z.object({
    streamId: z.string(),
    status: z.string(),
    recordingId: z.string().optional(),
    durationMs: z.number().optional(),
    code: z.string().optional(),
  }),
  "tts.start": z.object({ responseId: z.string(), format: audioFormatSchema }),
  "tts.end": z.object({
    responseId: z.string(),
    reason: z.enum(["completed", "barge_in", "cancelled", "error"]),
    code: z.string().optional(),
    chunks: z.number(),
  }),
} as const;

export type ServerMessageType = keyof typeof serverSchemas;
export type ServerPayload<T extends ServerMessageType> = z.infer<(typeof serverSchemas)[T]>;

/** A validated server message. `replyTo` is the client request it answers, if any. */
export type ServerMessage = {
  [T in ServerMessageType]: { type: T; replyTo: string | null; payload: ServerPayload<T> };
}[ServerMessageType];

export type ReadyPayload = ServerPayload<"ready">;
export type VoiceLimits = ReadyPayload["limits"];
export type VoiceSessionState = ServerPayload<"session.state">;
export type ConfirmRequestPayload = ServerPayload<"confirm.request">;
export type ActionResultPayload = ServerPayload<"action.result">;
export type ResponsePayload = ServerPayload<"response">;
export type AudioFormat = z.infer<typeof audioFormatSchema>;

const envelopeSchema = z.object({
  v: z.literal(PROTOCOL_VERSION),
  type: z.string(),
  replyTo: z.string().optional(),
  ts: z.number().optional(),
  payload: z.unknown().optional(),
});

export type ParseFailure = "not_json" | "bad_envelope" | "unknown_type" | "bad_payload";
export type ParseResult =
  | { ok: true; message: ServerMessage }
  | { ok: false; reason: ParseFailure };

/** Validates one text frame from the server. Never throws. */
export function parseServerMessage(raw: string): ParseResult {
  let json: unknown;
  try {
    json = JSON.parse(raw);
  } catch {
    return { ok: false, reason: "not_json" };
  }
  const env = envelopeSchema.safeParse(json);
  if (!env.success) return { ok: false, reason: "bad_envelope" };

  const type = env.data.type;
  if (!Object.prototype.hasOwnProperty.call(serverSchemas, type)) {
    return { ok: false, reason: "unknown_type" };
  }
  const schema = serverSchemas[type as ServerMessageType];
  const payload = schema.safeParse(env.data.payload ?? undefined);
  if (!payload.success) return { ok: false, reason: "bad_payload" };

  return {
    ok: true,
    message: {
      type,
      replyTo: env.data.replyTo ?? null,
      payload: payload.data,
    } as ServerMessage,
  };
}

/** Serializes a client message into the protocol envelope. */
export function encodeClientMessage<T extends ClientMessageType>(
  id: string,
  type: T,
  payload: ClientPayloads[T]
): string {
  return JSON.stringify(
    payload === undefined
      ? { v: PROTOCOL_VERSION, type, id }
      : { v: PROTOCOL_VERSION, type, id, payload }
  );
}

// ---------------------------------------------------------------------------
// Binary audio frames (both directions)
//
//   byte  0      frame version (1)
//   bytes 1-16   stream ID (client audio) or response ID (server audio), raw UUID
//   bytes 17-20  sequence number, big-endian uint32, starting at 0
//   bytes 21-    mono PCM16LE audio
// ---------------------------------------------------------------------------

export const FRAME_VERSION = 1;
export const FRAME_HEADER_BYTES = 21;

export interface AudioFrame {
  id: string;
  seq: number;
  audio: Uint8Array;
}

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function isUuid(value: string): boolean {
  return UUID_RE.test(value);
}

function uuidToBytes(uuid: string, out: Uint8Array, offset: number): void {
  const hex = uuid.replace(/-/g, "");
  for (let i = 0; i < 16; i++) {
    out[offset + i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  }
}

function bytesToUuid(bytes: Uint8Array, offset: number): string {
  let hex = "";
  for (let i = 0; i < 16; i++) {
    hex += bytes[offset + i].toString(16).padStart(2, "0");
  }
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

export function encodeAudioFrame(streamId: string, seq: number, pcm: Int16Array): ArrayBuffer {
  if (!isUuid(streamId)) throw new Error("stream id must be a UUID");
  const buffer = new ArrayBuffer(FRAME_HEADER_BYTES + pcm.length * 2);
  const bytes = new Uint8Array(buffer);
  const view = new DataView(buffer);
  bytes[0] = FRAME_VERSION;
  uuidToBytes(streamId, bytes, 1);
  view.setUint32(17, seq, false);
  for (let i = 0; i < pcm.length; i++) {
    view.setInt16(FRAME_HEADER_BYTES + i * 2, pcm[i], true);
  }
  return buffer;
}

/** Decodes a binary frame, or returns null if it is not a valid frame. */
export function decodeAudioFrame(data: ArrayBuffer): AudioFrame | null {
  if (data.byteLength < FRAME_HEADER_BYTES) return null;
  const bytes = new Uint8Array(data);
  if (bytes[0] !== FRAME_VERSION) return null;
  return {
    id: bytesToUuid(bytes, 1),
    seq: new DataView(data).getUint32(17, false),
    audio: bytes.subarray(FRAME_HEADER_BYTES),
  };
}

/** A random UUID for stream IDs; works without a secure context. */
export function newUuid(): string {
  const c = globalThis.crypto;
  if (c && typeof c.randomUUID === "function") return c.randomUUID();
  const b = new Uint8Array(16);
  if (c && typeof c.getRandomValues === "function") {
    c.getRandomValues(b);
  } else {
    for (let i = 0; i < 16; i++) b[i] = Math.floor(Math.random() * 256);
  }
  b[6] = (b[6] & 0x0f) | 0x40;
  b[8] = (b[8] & 0x3f) | 0x80;
  return bytesToUuid(b, 0);
}
