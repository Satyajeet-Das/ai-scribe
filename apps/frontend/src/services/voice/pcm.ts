/**
 * PCM helpers for turning microphone audio into the backend's wire format:
 * mono, signed 16-bit little-endian, at one of the sample rates it accepts.
 */

/** Sample rates the backend accepts, preferred first. 16 kHz is ample for speech. */
const PREFERRED_RATES = [16000, 8000, 24000, 48000];

/**
 * Picks the capture rate to send: the first preferred rate that the server
 * allows and that does not require upsampling the device's audio.
 */
export function pickSampleRate(deviceRate: number, allowed: readonly number[]): number | null {
  for (const rate of PREFERRED_RATES) {
    if (allowed.includes(rate) && rate <= deviceRate) return rate;
  }
  return null;
}

/**
 * Streaming downsampler. Each output sample is the average of the input
 * samples it spans, which also acts as a simple low-pass filter. State is kept
 * between calls so block boundaries do not produce clicks.
 */
export class Downsampler {
  private readonly ratio: number;
  /** Position of the next output sample's start, in input samples, relative to the next block. */
  private offset = 0;
  private carrySum = 0;
  private carryCount = 0;

  constructor(inputRate: number, outputRate: number) {
    if (outputRate > inputRate) throw new Error("upsampling is not supported");
    this.ratio = inputRate / outputRate;
  }

  process(input: Float32Array): Float32Array {
    if (this.ratio === 1) return input;

    const out = new Float32Array(Math.ceil((input.length + this.carryCount) / this.ratio) + 1);
    let n = 0;
    let sum = this.carrySum;
    let count = this.carryCount;
    // `end` is where the current output sample's window closes.
    let end = this.offset + this.ratio;

    for (let i = 0; i < input.length; i++) {
      sum += input[i];
      count++;
      if (i + 1 >= end) {
        out[n++] = sum / count;
        sum = 0;
        count = 0;
        end += this.ratio;
      }
    }

    this.carrySum = sum;
    this.carryCount = count;
    this.offset = end - this.ratio - input.length;
    return out.subarray(0, n);
  }
}

/** Converts float samples in [-1, 1] to 16-bit integers, clamping out-of-range values. */
export function floatToInt16(input: Float32Array): Int16Array {
  const out = new Int16Array(input.length);
  for (let i = 0; i < input.length; i++) {
    const s = Math.max(-1, Math.min(1, input[i]));
    out[i] = s < 0 ? Math.round(s * 0x8000) : Math.round(s * 0x7fff);
  }
  return out;
}

/** Converts PCM16LE bytes to float samples. A trailing odd byte is ignored. */
export function pcm16ToFloat(bytes: Uint8Array): Float32Array {
  const samples = bytes.byteLength >> 1;
  const view = new DataView(bytes.buffer, bytes.byteOffset, samples * 2);
  const out = new Float32Array(samples);
  for (let i = 0; i < samples; i++) {
    out[i] = view.getInt16(i * 2, true) / 0x8000;
  }
  return out;
}

/**
 * Groups samples into fixed-size chunks. It holds less than one chunk at any
 * time, so it cannot accumulate audio.
 */
export class PcmChunker {
  private buffer: Int16Array;
  private length = 0;

  constructor(private readonly samplesPerChunk: number) {
    if (samplesPerChunk <= 0) throw new Error("chunk size must be positive");
    this.buffer = new Int16Array(samplesPerChunk);
  }

  push(samples: Int16Array): Int16Array[] {
    const chunks: Int16Array[] = [];
    let i = 0;
    while (i < samples.length) {
      const take = Math.min(this.samplesPerChunk - this.length, samples.length - i);
      this.buffer.set(samples.subarray(i, i + take), this.length);
      this.length += take;
      i += take;
      if (this.length === this.samplesPerChunk) {
        chunks.push(this.buffer);
        this.buffer = new Int16Array(this.samplesPerChunk);
        this.length = 0;
      }
    }
    return chunks;
  }

  /** Returns whatever is left as a final, shorter chunk. */
  flush(): Int16Array | null {
    if (this.length === 0) return null;
    const rest = this.buffer.slice(0, this.length);
    this.length = 0;
    return rest;
  }

  reset(): void {
    this.length = 0;
  }
}
