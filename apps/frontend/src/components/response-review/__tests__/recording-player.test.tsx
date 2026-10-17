import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { RecordingPlayer, describePlaybackError } from "../recording-player";
import { ApiError } from "@/services/api";

const LABEL = "recording 1 of 1 for question 2";

let playSpy: ReturnType<typeof vi.fn>;
let pauseSpy: ReturnType<typeof vi.fn>;
let created: string[];
let revoked: string[];

beforeEach(() => {
  created = [];
  revoked = [];
  playSpy = vi.fn().mockResolvedValue(undefined);
  pauseSpy = vi.fn();
  // jsdom has no media playback.
  Object.defineProperty(HTMLMediaElement.prototype, "play", { configurable: true, value: playSpy });
  Object.defineProperty(HTMLMediaElement.prototype, "pause", { configurable: true, value: pauseSpy });
  let n = 0;
  URL.createObjectURL = vi.fn(() => {
    const url = `blob:audio-${++n}`;
    created.push(url);
    return url;
  });
  URL.revokeObjectURL = vi.fn((url: string) => {
    revoked.push(url);
  });
});

afterEach(() => vi.restoreAllMocks());

const audioEl = () => document.querySelector("audio") as HTMLAudioElement;
const blob = () => new Blob(["RIFF"], { type: "audio/wav" });

function setup(load = vi.fn().mockResolvedValue(blob())) {
  const view = render(<RecordingPlayer label={LABEL} durationMs={12_000} load={load} />);
  return { load, ...view };
}

async function press(name: RegExp | string) {
  await userEvent.click(screen.getByRole("button", { name }));
}

describe("RecordingPlayer", () => {
  it("fetches nothing until the reviewer asks to listen", () => {
    const { load } = setup();
    expect(load).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: `Play ${LABEL}` })).toBeInTheDocument();
    expect(screen.getByText("(0:12)")).toBeInTheDocument(); // length is known from the server
  });

  it("fetches once with the reviewer's request, then plays from memory", async () => {
    const { load } = setup();
    await press(`Play ${LABEL}`);

    await screen.findByRole("group", { name: LABEL });
    expect(load).toHaveBeenCalledTimes(1);
    expect(load.mock.calls[0][0]).toBeInstanceOf(AbortSignal);
    expect(audioEl().src).toBe(created[0]);
    expect(created[0]).toMatch(/^blob:/);
    expect(playSpy).toHaveBeenCalled();
    expect(screen.getByRole("status")).toHaveTextContent("Recording ready.");
  });

  it("pauses and resumes without fetching again", async () => {
    const { load } = setup();
    await press(`Play ${LABEL}`);
    await screen.findByRole("group", { name: LABEL });

    act(() => {
      audioEl().dispatchEvent(new Event("play"));
    });
    const pause = screen.getByRole("button", { name: `Pause ${LABEL}` });
    expect(pause).toHaveAttribute("aria-pressed", "true");

    Object.defineProperty(audioEl(), "paused", { configurable: true, value: false });
    pauseSpy.mockClear(); // starting a fetch pauses any earlier audio
    await userEvent.click(pause);
    expect(pauseSpy).toHaveBeenCalledTimes(1);

    act(() => {
      audioEl().dispatchEvent(new Event("pause"));
    });
    Object.defineProperty(audioEl(), "paused", { configurable: true, value: true });
    playSpy.mockClear();
    await userEvent.click(screen.getByRole("button", { name: `Play ${LABEL}` }));
    expect(playSpy).toHaveBeenCalledTimes(1);
    expect(load).toHaveBeenCalledTimes(1);
  });

  it("offers a labelled seek slider that follows playback and moves it", async () => {
    setup();
    await press(`Play ${LABEL}`);
    const slider = await screen.findByRole("slider", { name: `Seek ${LABEL}` });

    Object.defineProperty(audioEl(), "duration", { configurable: true, value: 12 });
    act(() => {
      audioEl().dispatchEvent(new Event("loadedmetadata"));
    });
    Object.defineProperty(audioEl(), "currentTime", { configurable: true, writable: true, value: 3 });
    act(() => {
      audioEl().dispatchEvent(new Event("timeupdate"));
    });
    expect(slider).toHaveAttribute("aria-valuetext", "0:03 of 0:12");

    fireEvent.change(slider, { target: { value: "8" } });
    expect(audioEl().currentTime).toBe(8);
    expect(slider).toHaveAttribute("aria-valuetext", "0:08 of 0:12");
  });

  it("skips back and forward five seconds without leaving the recording", async () => {
    setup();
    await press(`Play ${LABEL}`);
    await screen.findByRole("group", { name: LABEL });
    Object.defineProperty(audioEl(), "duration", { configurable: true, value: 12 });
    act(() => {
      audioEl().dispatchEvent(new Event("loadedmetadata"));
    });
    Object.defineProperty(audioEl(), "currentTime", { configurable: true, writable: true, value: 3 });
    act(() => {
      audioEl().dispatchEvent(new Event("timeupdate"));
    });

    await press("Forward 5 seconds");
    expect(audioEl().currentTime).toBe(8);
    await press("Forward 5 seconds");
    expect(audioEl().currentTime).toBe(12); // stops at the end
    await press("Back 5 seconds");
    expect(audioEl().currentTime).toBe(7);
    await press("Back 5 seconds");
    await press("Back 5 seconds");
    expect(audioEl().currentTime).toBe(0); // and at the start
  });

  it("changes the playback speed", async () => {
    setup();
    await press(`Play ${LABEL}`);
    await screen.findByRole("group", { name: LABEL });

    await userEvent.selectOptions(screen.getByRole("combobox", { name: /speed/i }), "1.5");
    expect(audioEl().playbackRate).toBe(1.5);
  });

  it("still shows the controls when the browser blocks automatic playback", async () => {
    playSpy.mockRejectedValue(new DOMException("blocked", "NotAllowedError"));
    setup();
    await press(`Play ${LABEL}`);
    expect(await screen.findByRole("group", { name: LABEL })).toBeInTheDocument();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("releases the audio from memory when it goes away", async () => {
    const { unmount } = setup();
    await press(`Play ${LABEL}`);
    await screen.findByRole("group", { name: LABEL });

    unmount();
    expect(revoked).toEqual(created);
    expect(pauseSpy).toHaveBeenCalled();
  });

  it("abandons a fetch that is still running when it goes away", async () => {
    let signal: AbortSignal | undefined;
    const load = vi.fn((s: AbortSignal) => {
      signal = s;
      return new Promise<Blob>(() => undefined);
    });
    const { unmount } = setup(load);
    await press(`Play ${LABEL}`);
    expect(screen.getByText("Loading recording…")).toBeInTheDocument();

    unmount();
    expect(signal?.aborted).toBe(true);
    expect(created).toEqual([]);
  });

  describe("when the recording cannot be played", () => {
    it("says a removed recording is gone, and offers no retry", async () => {
      setup(vi.fn().mockRejectedValue(new ApiError("gone", 410, "RECORDING_REMOVED")));
      await press(`Play ${LABEL}`);
      expect(await screen.findByRole("alert")).toHaveTextContent(/has been removed/i);
      expect(screen.queryByRole("button", { name: /try again/i })).toBeNull();
    });

    it("lets the reviewer retry a temporary failure", async () => {
      const load = vi
        .fn()
        .mockRejectedValueOnce(new ApiError("unavailable", 503, "AUDIO_UNAVAILABLE"))
        .mockResolvedValueOnce(blob());
      setup(load);
      await press(`Play ${LABEL}`);
      expect(await screen.findByRole("alert")).toHaveTextContent(/cannot be played right now/i);

      await press(/try again/i);
      await screen.findByRole("group", { name: LABEL });
      expect(load).toHaveBeenCalledTimes(2);
      expect(screen.queryByRole("alert")).toBeNull();
    });

    it("explains a permission problem without offering a pointless retry", async () => {
      setup(vi.fn().mockRejectedValue(new ApiError("no", 403)));
      await press(`Play ${LABEL}`);
      expect(await screen.findByRole("alert")).toHaveTextContent(/do not have permission/i);
      expect(screen.queryByRole("button", { name: /try again/i })).toBeNull();
    });

    it("reports a recording the browser cannot decode", async () => {
      setup();
      await press(`Play ${LABEL}`);
      await screen.findByRole("group", { name: LABEL });
      act(() => {
        audioEl().dispatchEvent(new Event("error"));
      });
      await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent(/could not play/i));
    });
  });
});

describe("describePlaybackError", () => {
  it.each([
    [410, false],
    [404, false],
    [403, false],
    [401, false],
    [0, true],
    [503, true],
    [500, true],
  ])("status %i → retryable %s", (status, retryable) => {
    expect(describePlaybackError(new ApiError("x", status)).retryable).toBe(retryable);
  });

  it("treats anything unexpected as a temporary failure", () => {
    expect(describePlaybackError(new Error("boom")).retryable).toBe(true);
  });
});
