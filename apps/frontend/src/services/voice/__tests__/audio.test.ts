import { beforeEach, describe, expect, it, vi } from "vitest";
import { MicCapture, MicError } from "../mic-capture";
import { PcmPlayer, type PlaybackState } from "../pcm-player";
import { FakeAudioContext, FakeMediaStream, domError } from "./fakes";

const Ctx = FakeAudioContext as unknown as typeof AudioContext;
const ALLOWED = [8000, 16000, 24000, 48000];

function micSetup(getUserMedia: () => Promise<FakeMediaStream>) {
  const chunks: Int16Array[] = [];
  const interrupted: string[] = [];
  const request = vi.fn(getUserMedia);
  const mic = new MicCapture({
    allowedSampleRates: ALLOWED,
    chunkMillis: 100,
    onChunk: (c) => chunks.push(c),
    onInterrupted: (k) => interrupted.push(k),
    mediaDevices: { getUserMedia: request as unknown as MediaDevices["getUserMedia"] },
    AudioContextCtor: Ctx,
  });
  return { mic, chunks, interrupted, request };
}

describe("MicCapture", () => {
  beforeEach(() => FakeAudioContext.reset());

  it("does not touch the microphone until start is called", () => {
    const { request } = micSetup(async () => new FakeMediaStream());
    expect(request).not.toHaveBeenCalled();
    expect(FakeAudioContext.instances).toHaveLength(0);
  });

  it("produces 100 ms mono PCM16 chunks at 16 kHz from a 48 kHz device", async () => {
    const { mic, chunks } = micSetup(async () => new FakeMediaStream());
    await mic.start();
    expect(mic.getSampleRate()).toBe(16000);

    const ctx = FakeAudioContext.instances[0];
    // 0.25 s of audio at 48 kHz, in 4096-sample blocks.
    for (let sent = 0; sent < 12000; sent += 4096) {
      ctx.feed(new Float32Array(Math.min(4096, 12000 - sent)).fill(0.5));
    }

    expect(chunks).toHaveLength(2);
    expect(chunks.every((c) => c.length === 1600)).toBe(true);
    expect(chunks[0][0]).toBe(16384);

    // The unfinished chunk is delivered on a flushing stop, and only then.
    mic.stop(true);
    expect(chunks).toHaveLength(3);
    expect(chunks[2].length).toBe(800);
  });

  it.each([
    ["NotAllowedError", "denied"],
    ["SecurityError", "denied"],
    ["NotFoundError", "no_device"],
    ["NotReadableError", "failed"],
  ])("maps a %s from the browser to %s and holds nothing", async (name, kind) => {
    const { mic } = micSetup(async () => {
      throw domError(name);
    });
    await expect(mic.start()).rejects.toMatchObject({ kind });
    expect(mic.isActive()).toBe(false);
    expect(FakeAudioContext.instances).toHaveLength(0);
  });

  it("reports unsupported browsers", async () => {
    const mic = new MicCapture({
      allowedSampleRates: ALLOWED,
      chunkMillis: 100,
      onChunk: () => {},
      onInterrupted: () => {},
      mediaDevices: undefined,
      AudioContextCtor: undefined,
    });
    // jsdom has neither getUserMedia nor AudioContext.
    await expect(mic.start()).rejects.toBeInstanceOf(MicError);
    await expect(mic.start()).rejects.toMatchObject({ kind: "unsupported" });
  });

  it("releases the track, the nodes and the audio context on stop", async () => {
    const stream = new FakeMediaStream();
    const { mic, chunks } = micSetup(async () => stream);
    await mic.start();
    const ctx = FakeAudioContext.instances[0];
    expect(stream.tracks[0].listenerCount("ended")).toBe(1);

    mic.stop();

    expect(stream.tracks[0].stopped).toBe(true);
    expect(stream.tracks[0].listenerCount("ended")).toBe(0);
    expect(ctx.close).toHaveBeenCalledTimes(1);
    expect(ctx.nodes.every((n) => n.disconnected)).toBe(true);
    expect(ctx.processor?.onaudioprocess).toBeNull();
    expect(mic.isActive()).toBe(false);

    // Audio that arrives after stop is discarded.
    ctx.feed(new Float32Array(48000).fill(0.5));
    expect(chunks).toHaveLength(0);
  });

  it("releases the microphone if stop is called while the permission prompt is open", async () => {
    const stream = new FakeMediaStream();
    let grant: (s: FakeMediaStream) => void = () => {};
    const { mic } = micSetup(() => new Promise<FakeMediaStream>((r) => (grant = r)));

    const starting = mic.start();
    mic.stop();
    grant(stream);

    await expect(starting).rejects.toBeInstanceOf(MicError);
    expect(stream.tracks[0].stopped).toBe(true);
    expect(mic.isActive()).toBe(false);
    expect(FakeAudioContext.instances).toHaveLength(0);
  });

  it("stops and reports when the device goes away", async () => {
    const stream = new FakeMediaStream();
    const { mic, interrupted } = micSetup(async () => stream);
    await mic.start();

    stream.tracks[0].end();

    expect(interrupted).toEqual(["failed"]);
    expect(mic.isActive()).toBe(false);
    expect(FakeAudioContext.instances[0].close).toHaveBeenCalled();
  });

  it("can be started again after stopping", async () => {
    const { mic, request } = micSetup(async () => new FakeMediaStream());
    await mic.start();
    await mic.start();
    expect(request).toHaveBeenCalledTimes(1);
    mic.stop();
    await mic.start();
    expect(request).toHaveBeenCalledTimes(2);
    expect(mic.isActive()).toBe(true);
    mic.stop();
  });
});

const R1 = "11111111-1111-4111-8111-111111111111";
const R2 = "22222222-2222-4222-8222-222222222222";
const FORMAT = { encoding: "pcm_s16le", sampleRate: 24000, channels: 1 };
/** 100 ms of audio at 24 kHz. */
const chunk = () => new Uint8Array(4800);

function playerSetup() {
  const states: PlaybackState[] = [];
  const player = new PcmPlayer({
    onState: (s) => states.push(s),
    AudioContextCtor: Ctx,
    maxSeconds: 1,
  });
  return { player, states, ctx: () => FakeAudioContext.instances[0] };
}

describe("PcmPlayer", () => {
  beforeEach(() => FakeAudioContext.reset());

  it("schedules chunks back to back, in order, without overlap", async () => {
    const { player, states, ctx } = playerSetup();
    await player.unlock();
    player.begin(R1, FORMAT);

    expect(player.push(R1, 0, chunk())).toBe(true);
    expect(player.push(R1, 1, chunk())).toBe(true);
    expect(player.push(R1, 2, chunk())).toBe(true);

    const starts = ctx().sources.map((s) => s.startedAt!);
    expect(starts[1] - starts[0]).toBeCloseTo(0.1);
    expect(starts[2] - starts[1]).toBeCloseTo(0.1);
    expect(ctx().sources[0].buffer).toMatchObject({ length: 2400 });
    expect(states).toEqual(["playing"]);

    // Idle only after the stream has ended and every chunk has finished.
    player.end(R1);
    ctx().sources[0].finish();
    ctx().sources[1].finish();
    expect(states).toEqual(["playing"]);
    ctx().sources[2].finish();
    expect(states).toEqual(["playing", "idle"]);
  });

  it("rejects chunks for any response other than the active one", async () => {
    const { player, ctx } = playerSetup();
    await player.unlock();

    expect(player.push(R1, 0, chunk())).toBe(false); // nothing begun
    player.begin(R1, FORMAT);
    expect(player.push(R2, 0, chunk())).toBe(false);
    expect(ctx().sources).toHaveLength(0);
  });

  it("rejects duplicate and out-of-order chunks", async () => {
    const { player, ctx } = playerSetup();
    await player.unlock();
    player.begin(R1, FORMAT);

    expect(player.push(R1, 0, chunk())).toBe(true);
    expect(player.push(R1, 0, chunk())).toBe(false);
    expect(player.push(R1, 2, chunk())).toBe(false);
    expect(player.push(R1, 1, chunk())).toBe(true);
    expect(ctx().sources).toHaveLength(2);
  });

  it("stops immediately and rejects late chunks of the stopped response", async () => {
    const { player, states, ctx } = playerSetup();
    await player.unlock();
    player.begin(R1, FORMAT);
    player.push(R1, 0, chunk());
    player.push(R1, 1, chunk());

    player.stop();

    expect(ctx().sources.every((s) => s.stopped && s.disconnected && s.onended === null)).toBe(
      true
    );
    expect(states).toEqual(["playing", "idle"]);
    expect(player.push(R1, 2, chunk())).toBe(false);
    player.end(R1);
    expect(ctx().sources).toHaveLength(2);
  });

  it("replaces the current response when a new one begins", async () => {
    const { player, ctx } = playerSetup();
    await player.unlock();
    player.begin(R1, FORMAT);
    player.push(R1, 0, chunk());
    player.begin(R2, FORMAT);

    expect(ctx().sources[0].stopped).toBe(true);
    expect(player.push(R1, 1, chunk())).toBe(false);
    expect(player.push(R2, 0, chunk())).toBe(true);
    expect(player.getActiveId()).toBe(R2);
  });

  it("refuses formats the protocol does not define", async () => {
    const { player } = playerSetup();
    await player.unlock();
    expect(player.begin(R1, { ...FORMAT, encoding: "mp3" })).toBe(false);
    expect(player.begin(R1, { ...FORMAT, channels: 2 })).toBe(false);
    expect(player.push(R1, 0, chunk())).toBe(false);
  });

  it("bounds how much audio one response may hold", async () => {
    const { player } = playerSetup(); // maxSeconds: 1
    await player.unlock();
    player.begin(R1, FORMAT);
    let accepted = 0;
    for (let seq = 0; seq < 50; seq++) {
      if (player.push(R1, seq, chunk())) accepted++;
    }
    expect(accepted).toBe(10);
  });

  it("reports blocked audio under autoplay restrictions and plays on replay once unlocked", async () => {
    FakeAudioContext.initialState = "suspended";
    FakeAudioContext.resumable = false;
    const { player, states, ctx } = playerSetup();
    expect(await player.unlock()).toBe(false);

    player.begin(R1, FORMAT);
    expect(player.push(R1, 0, chunk())).toBe(true);
    player.end(R1);
    expect(states).toEqual(["blocked"]);
    expect(ctx().sources).toHaveLength(0);
    expect(player.canReplay()).toBe(true);

    // A user gesture now allows audio.
    FakeAudioContext.resumable = true;
    expect(await player.replay()).toBe(true);
    expect(ctx().sources).toHaveLength(1);
    expect(states).toEqual(["blocked", "playing"]);
    ctx().sources[0].finish();
    expect(states).toEqual(["blocked", "playing", "idle"]);
  });

  it("replays the last response from memory and has nothing to replay initially", async () => {
    const { player, ctx } = playerSetup();
    await player.unlock();
    expect(await player.replay()).toBe(false);

    player.begin(R1, FORMAT);
    player.push(R1, 0, chunk());
    player.push(R1, 1, chunk());
    player.end(R1);
    ctx().sources.forEach((s) => s.finish());

    expect(await player.replay()).toBe(true);
    expect(ctx().sources).toHaveLength(4);
  });

  it("releases its sources, buffers and audio context on dispose", async () => {
    const { player, states, ctx } = playerSetup();
    await player.unlock();
    player.begin(R1, FORMAT);
    player.push(R1, 0, chunk());

    player.dispose();

    expect(ctx().sources[0].stopped).toBe(true);
    expect(ctx().close).toHaveBeenCalledTimes(1);
    expect(player.canReplay()).toBe(false);
    expect(states[states.length - 1]).toBe("idle");
  });

  it("works without any audio output device API", async () => {
    const states: PlaybackState[] = [];
    const player = new PcmPlayer({ onState: (s) => states.push(s), AudioContextCtor: undefined });
    expect(await player.unlock()).toBe(false);
    player.begin(R1, FORMAT);
    expect(player.push(R1, 0, chunk())).toBe(true);
    expect(states).toEqual(["blocked"]);
  });
});
