import { describe, expect, it } from "vitest";
import {
  FRAME_HEADER_BYTES,
  decodeAudioFrame,
  encodeAudioFrame,
  encodeClientMessage,
  isUuid,
  newUuid,
  parseServerMessage,
} from "../protocol";
import { Downsampler, PcmChunker, floatToInt16, pcm16ToFloat, pickSampleRate } from "../pcm";
import { voiceSocketUrl } from "../voice-socket";
import { QUESTION_1, readyPayload } from "./fakes";

const envelope = (type: string, payload?: unknown, extra: Record<string, unknown> = {}) =>
  JSON.stringify({ v: 1, type, ts: 1, payload, ...extra });

describe("voice protocol: incoming message validation", () => {
  it("accepts every server message the backend defines", () => {
    const samples: Array<[string, unknown]> = [
      ["ready", readyPayload()],
      ["ack", undefined],
      ["pong", undefined],
      ["error", { code: "stt_failed", message: "x", retryable: true, streamId: newUuid() }],
      ["session.state", { status: "IN_PROGRESS", remainingSeconds: 10, question: QUESTION_1 }],
      ["session.closed", { reason: "expired" }],
      ["auth.expiring", { expiresAt: "2026-10-12T10:00:00Z" }],
      ["stt.partial", { streamId: newUuid(), text: "opt" }],
      ["stt.final", { streamId: newUuid(), text: "option B" }],
      ["intent", { intent: "select_option", optionKey: "B", confidence: 1, classifier: "rules" }],
      [
        "confirm.request",
        {
          confirmationId: "c1",
          action: "select_option",
          questionId: QUESTION_1.id,
          optionKey: "B",
          expiresAt: "2026-10-12T10:00:00Z",
        },
      ],
      ["action.result", { action: "answer", status: "ok", saved: true, questionId: QUESTION_1.id }],
      ["response", { responseId: newUuid(), kind: "question", text: "Question 1", audio: true }],
      [
        "tts.start",
        {
          responseId: newUuid(),
          format: { encoding: "pcm_s16le", sampleRate: 24000, channels: 1 },
        },
      ],
      ["tts.end", { responseId: newUuid(), reason: "completed", chunks: 4 }],
    ];
    for (const [type, payload] of samples) {
      const result = parseServerMessage(envelope(type, payload, { replyTo: "c1-x" }));
      expect(result.ok, type).toBe(true);
      if (result.ok) {
        expect(result.message.type).toBe(type);
        expect(result.message.replyTo).toBe("c1-x");
      }
    }
  });

  it("rejects malformed and unexpected messages without throwing", () => {
    expect(parseServerMessage("{not json")).toEqual({ ok: false, reason: "not_json" });
    expect(parseServerMessage("null")).toEqual({ ok: false, reason: "bad_envelope" });
    expect(parseServerMessage(JSON.stringify({ type: "ack" }))).toEqual({
      ok: false,
      reason: "bad_envelope",
    });
    expect(parseServerMessage(envelope("ack", undefined, { v: 2 }))).toEqual({
      ok: false,
      reason: "bad_envelope",
    });
    expect(parseServerMessage(envelope("exam.reveal_answers", {}))).toEqual({
      ok: false,
      reason: "unknown_type",
    });
    // Prototype keys must not be mistaken for message types.
    expect(parseServerMessage(envelope("constructor", {}))).toEqual({
      ok: false,
      reason: "unknown_type",
    });
    expect(parseServerMessage(envelope("stt.final", { streamId: 5 }))).toEqual({
      ok: false,
      reason: "bad_payload",
    });
    expect(
      parseServerMessage(envelope("action.result", { action: "grade", status: "ok", saved: true }))
    ).toEqual({
      ok: false,
      reason: "bad_payload",
    });
    expect(
      parseServerMessage(
        envelope("action.result", { action: "answer", status: "ok", saved: "yes" })
      )
    ).toEqual({
      ok: false,
      reason: "bad_payload",
    });
    expect(
      parseServerMessage(envelope("tts.end", { responseId: "r", reason: "exploded", chunks: 1 }))
    ).toEqual({
      ok: false,
      reason: "bad_payload",
    });
    expect(parseServerMessage(envelope("ready", { ...readyPayload(), limits: undefined }))).toEqual(
      {
        ok: false,
        reason: "bad_payload",
      }
    );
  });

  it("drops fields the protocol does not define, such as an answer key", () => {
    const leaky = {
      ...QUESTION_1,
      options: QUESTION_1.options.map((o) => ({ ...o, isCorrect: true })),
    };
    const result = parseServerMessage(
      envelope("session.state", { status: "IN_PROGRESS", question: leaky })
    );
    expect(result.ok).toBe(true);
    expect(JSON.stringify(result)).not.toContain("isCorrect");
  });
});

describe("voice protocol: outgoing messages and audio frames", () => {
  it("wraps client messages in the versioned envelope", () => {
    expect(JSON.parse(encodeClientMessage("c1", "ping", undefined))).toEqual({
      v: 1,
      type: "ping",
      id: "c1",
    });
    expect(
      JSON.parse(encodeClientMessage("c2", "command", { intent: "confirm", confirmationId: "x" }))
    ).toEqual({
      v: 1,
      type: "command",
      id: "c2",
      payload: { intent: "confirm", confirmationId: "x" },
    });
  });

  it("encodes audio frames as version, stream id, big-endian sequence, PCM16LE", () => {
    const id = "00112233-4455-4677-8899-aabbccddeeff";
    const frame = encodeAudioFrame(id, 258, new Int16Array([1, -2, 0x1234]));
    const bytes = new Uint8Array(frame);

    expect(bytes.length).toBe(FRAME_HEADER_BYTES + 6);
    expect(bytes[0]).toBe(1);
    expect(Array.from(bytes.slice(1, 17))).toEqual([
      0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee,
      0xff,
    ]);
    expect(Array.from(bytes.slice(17, 21))).toEqual([0, 0, 1, 2]);
    expect(Array.from(bytes.slice(21))).toEqual([0x01, 0x00, 0xfe, 0xff, 0x34, 0x12]);

    const decoded = decodeAudioFrame(frame);
    expect(decoded).toMatchObject({ id, seq: 258 });
    expect(decoded!.audio.length).toBe(6);
  });

  it("rejects frames that are too short, the wrong version, or carry a non-UUID id", () => {
    expect(decodeAudioFrame(new ArrayBuffer(5))).toBeNull();
    const wrongVersion = new Uint8Array(FRAME_HEADER_BYTES + 2);
    wrongVersion[0] = 9;
    expect(decodeAudioFrame(wrongVersion.buffer)).toBeNull();
    expect(() => encodeAudioFrame("not-a-uuid", 0, new Int16Array(1))).toThrow();
  });

  it("generates distinct UUIDs", () => {
    const a = newUuid();
    expect(isUuid(a)).toBe(true);
    expect(newUuid()).not.toBe(a);
  });

  it("builds the socket URL from the API base without credentials or a query string", () => {
    expect(voiceSocketUrl("http://localhost:8080/api/v1", "abc")).toBe(
      "ws://localhost:8080/api/v1/sessions/abc/voice"
    );
    expect(voiceSocketUrl("https://api.example.com/api/v1/", "abc")).toBe(
      "wss://api.example.com/api/v1/sessions/abc/voice"
    );
    expect(voiceSocketUrl("/api/v1", "abc", "https://exam.example.com")).toBe(
      "wss://exam.example.com/api/v1/sessions/abc/voice"
    );
    const stripped = voiceSocketUrl("https://user:secret@api.example.com/api/v1?token=leak", "a/b");
    expect(stripped).not.toContain("secret");
    expect(stripped).not.toContain("token");
    expect(stripped).toContain("/sessions/a%2Fb/voice");
  });
});

describe("PCM helpers", () => {
  it("picks a capture rate the server allows without upsampling", () => {
    const allowed = [8000, 16000, 24000, 48000];
    expect(pickSampleRate(48000, allowed)).toBe(16000);
    expect(pickSampleRate(44100, allowed)).toBe(16000);
    expect(pickSampleRate(12000, allowed)).toBe(8000);
    expect(pickSampleRate(48000, [48000])).toBe(48000);
    expect(pickSampleRate(4000, allowed)).toBeNull();
  });

  it("downsamples to the exact output rate across uneven blocks", () => {
    for (const inputRate of [48000, 44100]) {
      const down = new Downsampler(inputRate, 16000);
      let produced = 0;
      // One second of audio in 128-sample render quanta.
      for (let sent = 0; sent < inputRate; sent += 128) {
        produced += down.process(
          new Float32Array(Math.min(128, inputRate - sent)).fill(0.5)
        ).length;
      }
      expect(Math.abs(produced - 16000)).toBeLessThanOrEqual(1);
    }
  });

  it("averages the samples each output sample spans, carrying state between calls", () => {
    const down = new Downsampler(48000, 16000);
    const first = down.process(new Float32Array([0.3, 0.3, 0.3, 0.9]));
    const second = down.process(new Float32Array([0.9, 0.9]));
    expect(Array.from(first)).toHaveLength(1);
    expect(first[0]).toBeCloseTo(0.3);
    expect(second[0]).toBeCloseTo(0.9);
    expect(new Downsampler(16000, 16000).process(new Float32Array([0.1])).length).toBe(1);
    expect(() => new Downsampler(8000, 16000)).toThrow();
  });

  it("converts between float and 16-bit samples, clamping overflow", () => {
    expect(Array.from(floatToInt16(new Float32Array([0, 1, -1, 2, -2, 0.5])))).toEqual([
      0, 32767, -32768, 32767, -32768, 16384,
    ]);
    const back = pcm16ToFloat(new Uint8Array([0x00, 0x40, 0x00, 0xc0, 0x7f]));
    expect(back.length).toBe(2);
    expect(back[0]).toBeCloseTo(0.5);
    expect(back[1]).toBeCloseTo(-0.5);
  });

  it("chunks samples without holding more than one chunk", () => {
    const chunker = new PcmChunker(4);
    expect(chunker.push(new Int16Array([1, 2, 3]))).toEqual([]);
    const chunks = chunker.push(new Int16Array([4, 5, 6, 7, 8, 9]));
    expect(chunks.map((c) => Array.from(c))).toEqual([
      [1, 2, 3, 4],
      [5, 6, 7, 8],
    ]);
    expect(Array.from(chunker.flush()!)).toEqual([9]);
    expect(chunker.flush()).toBeNull();
  });
});
