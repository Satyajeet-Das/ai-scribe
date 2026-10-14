/**
 * Microphone capture. Produces mono PCM16 chunks at the requested sample rate
 * and holds no audio beyond the chunk being filled. Every resource it takes
 * (tracks, audio context, nodes) is released on stop.
 */
import { Downsampler, PcmChunker, floatToInt16, pickSampleRate } from "./pcm";

export type MicErrorKind = "unsupported" | "denied" | "no_device" | "failed";

export class MicError extends Error {
  constructor(public readonly kind: MicErrorKind) {
    super(`microphone ${kind}`);
    this.name = "MicError";
  }
}

export interface MicCaptureOptions {
  /** Sample rates the server accepts. */
  allowedSampleRates: readonly number[];
  /** Length of each chunk in milliseconds. */
  chunkMillis: number;
  /** Receives each chunk as soon as it is full. */
  onChunk(pcm: Int16Array): void;
  /** Called when capture stops by itself (device unplugged, track ended). */
  onInterrupted(kind: MicErrorKind): void;
  mediaDevices?: Pick<MediaDevices, "getUserMedia">;
  AudioContextCtor?: typeof AudioContext;
}

const WORKLET_NAME = "scribe-pcm-tap";
const WORKLET_SOURCE = `
class ScribePcmTap extends AudioWorkletProcessor {
  process(inputs) {
    const channel = inputs[0] && inputs[0][0];
    if (channel) this.port.postMessage(channel.slice(0));
    return true;
  }
}
registerProcessor("${WORKLET_NAME}", ScribePcmTap);
`;

function audioContextCtor(): typeof AudioContext | undefined {
  if (typeof window === "undefined") return undefined;
  const w = window as unknown as {
    AudioContext?: typeof AudioContext;
    webkitAudioContext?: typeof AudioContext;
  };
  return w.AudioContext ?? w.webkitAudioContext;
}

export function isMicSupported(): boolean {
  return (
    typeof navigator !== "undefined" &&
    !!navigator.mediaDevices &&
    typeof navigator.mediaDevices.getUserMedia === "function" &&
    !!audioContextCtor()
  );
}

function toMicError(err: unknown): MicError {
  const name = err instanceof Error ? err.name : "";
  if (name === "NotAllowedError" || name === "SecurityError" || name === "PermissionDeniedError") {
    return new MicError("denied");
  }
  if (
    name === "NotFoundError" ||
    name === "DevicesNotFoundError" ||
    name === "OverconstrainedError"
  ) {
    return new MicError("no_device");
  }
  return new MicError("failed");
}

export class MicCapture {
  private stream: MediaStream | null = null;
  private context: AudioContext | null = null;
  private nodes: AudioNode[] = [];
  private chunker: PcmChunker | null = null;
  private sampleRate = 0;
  /** Bumped on every start and stop so a slow permission prompt cannot revive a stopped capture. */
  private run = 0;
  private active = false;

  constructor(private readonly opts: MicCaptureOptions) {}

  isActive(): boolean {
    return this.active;
  }

  /** The sample rate of the chunks being produced; valid after start() resolves. */
  getSampleRate(): number {
    return this.sampleRate;
  }

  /**
   * Asks for the microphone and starts producing chunks. Call it only from an
   * explicit user action: this is what triggers the browser's permission prompt.
   */
  async start(): Promise<void> {
    if (this.active) return;
    const devices =
      this.opts.mediaDevices ??
      (typeof navigator !== "undefined" ? navigator.mediaDevices : undefined);
    const Ctor = this.opts.AudioContextCtor ?? audioContextCtor();
    if (!devices || typeof devices.getUserMedia !== "function" || !Ctor) {
      throw new MicError("unsupported");
    }

    const run = ++this.run;
    let stream: MediaStream;
    try {
      stream = await devices.getUserMedia({
        audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true },
      });
    } catch (err) {
      throw toMicError(err);
    }
    if (run !== this.run) {
      // stop() was called while the permission prompt was open.
      stream.getTracks().forEach((t) => t.stop());
      throw new MicError("failed");
    }

    this.stream = stream;
    try {
      const context = new Ctor();
      this.context = context;
      if (context.state === "suspended") await context.resume();

      const rate = pickSampleRate(context.sampleRate, this.opts.allowedSampleRates);
      if (rate === null) throw new MicError("unsupported");
      this.sampleRate = rate;

      const downsampler = new Downsampler(context.sampleRate, rate);
      const chunker = new PcmChunker(
        Math.max(1, Math.round((rate * this.opts.chunkMillis) / 1000))
      );
      this.chunker = chunker;
      const onSamples = (samples: Float32Array) => {
        if (run !== this.run || !this.active) return;
        for (const chunk of chunker.push(floatToInt16(downsampler.process(samples)))) {
          this.opts.onChunk(chunk);
        }
      };

      const source = context.createMediaStreamSource(stream);
      const tap = await this.createTap(context, onSamples);
      if (run !== this.run) throw new MicError("failed");

      // The tap must reach the destination to be processed; the gain keeps it silent.
      const mute = context.createGain();
      mute.gain.value = 0;
      source.connect(tap);
      tap.connect(mute);
      mute.connect(context.destination);
      this.nodes = [source, tap, mute];

      for (const track of stream.getAudioTracks()) {
        track.addEventListener("ended", this.handleTrackEnded);
      }
      this.active = true;
    } catch (err) {
      this.release();
      throw err instanceof MicError ? err : toMicError(err);
    }
  }

  /**
   * Stops capturing and releases the microphone. With `flush`, the samples
   * still held for the unfinished chunk are delivered first.
   */
  stop(flush = false): void {
    const wasActive = this.active;
    this.active = false;
    this.run++;
    if (wasActive && flush) {
      const rest = this.chunker?.flush();
      if (rest && rest.length > 0) this.opts.onChunk(rest);
    }
    this.release();
  }

  private async createTap(
    context: AudioContext,
    onSamples: (samples: Float32Array) => void
  ): Promise<AudioNode> {
    if (context.audioWorklet && typeof AudioWorkletNode !== "undefined") {
      const url = URL.createObjectURL(
        new Blob([WORKLET_SOURCE], { type: "application/javascript" })
      );
      try {
        await context.audioWorklet.addModule(url);
      } finally {
        URL.revokeObjectURL(url);
      }
      const node = new AudioWorkletNode(context, WORKLET_NAME, {
        numberOfInputs: 1,
        numberOfOutputs: 1,
        channelCount: 1,
      });
      node.port.onmessage = (event: MessageEvent) => onSamples(event.data as Float32Array);
      return node;
    }
    // Older browsers without AudioWorklet.
    const node = context.createScriptProcessor(4096, 1, 1);
    node.onaudioprocess = (event: AudioProcessingEvent) => {
      onSamples(new Float32Array(event.inputBuffer.getChannelData(0)));
    };
    return node;
  }

  private handleTrackEnded = (): void => {
    if (!this.active) return;
    this.stop(false);
    this.opts.onInterrupted("failed");
  };

  private release(): void {
    for (const node of this.nodes) {
      try {
        node.disconnect();
      } catch {
        // already disconnected
      }
      if ("port" in node) (node as AudioWorkletNode).port.onmessage = null;
      if ("onaudioprocess" in node) (node as ScriptProcessorNode).onaudioprocess = null;
    }
    this.nodes = [];

    if (this.stream) {
      for (const track of this.stream.getTracks()) {
        track.removeEventListener("ended", this.handleTrackEnded);
        track.stop();
      }
      this.stream = null;
    }
    if (this.context) {
      const context = this.context;
      this.context = null;
      if (context.state !== "closed") void context.close().catch(() => {});
    }
    this.chunker?.reset();
    this.chunker = null;
  }
}
