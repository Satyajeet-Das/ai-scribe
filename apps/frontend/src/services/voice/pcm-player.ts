/**
 * Plays the server's streamed speech. Audio arrives as PCM16 chunks tagged
 * with a response ID and a sequence number. Only one response plays at a time;
 * chunks for any other response, or out of order, are rejected.
 */
import { PCM_ENCODING, type AudioFormat } from "./protocol";
import { pcm16ToFloat } from "./pcm";

export type PlaybackState = "idle" | "playing" | "blocked";

export interface PcmPlayerOptions {
  onState(state: PlaybackState): void;
  /** Upper bound on audio kept for one response, in seconds. */
  maxSeconds?: number;
  AudioContextCtor?: typeof AudioContext;
}

function defaultContextCtor(): typeof AudioContext | undefined {
  if (typeof window === "undefined") return undefined;
  const w = window as unknown as {
    AudioContext?: typeof AudioContext;
    webkitAudioContext?: typeof AudioContext;
  };
  return w.AudioContext ?? w.webkitAudioContext;
}

export class PcmPlayer {
  private context: AudioContext | null = null;
  private state: PlaybackState = "idle";

  private activeId: string | null = null;
  private format: AudioFormat | null = null;
  private nextSeq = 0;
  private ended = false;
  private nextStart = 0;
  private sources = new Set<AudioBufferSourceNode>();

  /** The current (or last finished) response, kept so it can be replayed. */
  private clips: Float32Array[] = [];
  private clipSeconds = 0;
  private clipFormat: AudioFormat | null = null;
  private readonly maxSeconds: number;

  constructor(private readonly opts: PcmPlayerOptions) {
    this.maxSeconds = opts.maxSeconds ?? 120;
  }

  getState(): PlaybackState {
    return this.state;
  }

  /** The response currently accepted for playback, if any. */
  getActiveId(): string | null {
    return this.activeId;
  }

  canReplay(): boolean {
    return this.clips.length > 0;
  }

  /**
   * Creates and resumes the audio context. Browsers only allow this from a
   * user gesture, so it is called from the button that enables voice.
   */
  async unlock(): Promise<boolean> {
    const context = this.ensureContext();
    if (!context) return false;
    if (context.state === "suspended") {
      try {
        await context.resume();
      } catch {
        return false;
      }
    }
    return context.state === "running";
  }

  /** Starts a new response, replacing whatever was playing. */
  begin(responseId: string, format: AudioFormat): boolean {
    this.halt();
    if (format.encoding !== PCM_ENCODING || format.channels !== 1) return false;
    this.activeId = responseId;
    this.format = format;
    this.nextSeq = 0;
    this.ended = false;
    this.clips = [];
    this.clipSeconds = 0;
    this.clipFormat = format;
    return true;
  }

  /** Queues one chunk. Returns false if the chunk was rejected. */
  push(responseId: string, seq: number, audio: Uint8Array): boolean {
    // Stale: from a response that was cancelled, replaced, or never accepted.
    if (responseId !== this.activeId || !this.format) return false;
    // The transport is ordered, so anything but the next number is a duplicate
    // or a replay and is never played.
    if (seq !== this.nextSeq) return false;
    this.nextSeq++;

    const samples = pcm16ToFloat(audio);
    if (samples.length === 0) return true;
    const seconds = samples.length / this.format.sampleRate;
    if (this.clipSeconds + seconds > this.maxSeconds) return false;
    this.clips.push(samples);
    this.clipSeconds += seconds;

    this.schedule(samples, this.format);
    return true;
  }

  /** Marks the response as fully received. */
  end(responseId: string): void {
    if (responseId !== this.activeId) return;
    this.ended = true;
    this.settle();
  }

  /** Stops playback immediately. Chunks that arrive afterwards for it are rejected. */
  stop(): void {
    this.halt();
    this.setState("idle");
  }

  /** Plays the last response again from memory. */
  async replay(): Promise<boolean> {
    if (this.clips.length === 0 || !this.clipFormat) return false;
    const clips = this.clips;
    const format = this.clipFormat;
    this.halt();
    if (!(await this.unlock())) {
      this.setState("blocked");
      return false;
    }
    this.ended = true;
    for (const clip of clips) this.schedule(clip, format);
    return true;
  }

  dispose(): void {
    this.halt();
    this.clips = [];
    this.clipSeconds = 0;
    this.setState("idle");
    if (this.context) {
      const context = this.context;
      this.context = null;
      if (context.state !== "closed") void context.close().catch(() => {});
    }
  }

  // -------------------------------------------------------------------------

  private ensureContext(): AudioContext | null {
    if (this.context) return this.context;
    const Ctor = this.opts.AudioContextCtor ?? defaultContextCtor();
    if (!Ctor) return null;
    try {
      this.context = new Ctor();
    } catch {
      this.context = null;
    }
    return this.context;
  }

  /** Stops every scheduled source and forgets the active response. */
  private halt(): void {
    this.activeId = null;
    this.format = null;
    this.ended = false;
    this.nextStart = 0;
    const sources = this.sources;
    this.sources = new Set();
    for (const source of sources) {
      source.onended = null;
      try {
        source.stop();
      } catch {
        // not started yet
      }
      source.disconnect();
    }
  }

  private schedule(samples: Float32Array, format: AudioFormat): void {
    const context = this.ensureContext();
    if (!context || context.state !== "running") {
      // Autoplay policy, or no audio device. The text of the response is
      // still shown; the audio stays available for replay once unblocked.
      if (context && context.state === "suspended") void context.resume().catch(() => {});
      this.setState("blocked");
      return;
    }

    const buffer = context.createBuffer(1, samples.length, format.sampleRate);
    buffer.getChannelData(0).set(samples);
    const source = context.createBufferSource();
    source.buffer = buffer;
    source.connect(context.destination);

    // Chunks are scheduled back to back, in arrival order, so they never overlap.
    const startAt = Math.max(context.currentTime + 0.03, this.nextStart);
    this.nextStart = startAt + buffer.duration;
    this.sources.add(source);
    source.onended = () => {
      this.sources.delete(source);
      source.disconnect();
      this.settle();
    };
    source.start(startAt);
    this.setState("playing");
  }

  private settle(): void {
    if (this.ended && this.sources.size === 0 && this.state === "playing") {
      this.setState("idle");
    }
  }

  private setState(state: PlaybackState): void {
    if (this.state === state) return;
    this.state = state;
    this.opts.onState(state);
  }
}
