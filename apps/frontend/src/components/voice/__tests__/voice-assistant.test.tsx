import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { VoiceAssistant } from "../voice-assistant";
import type { VoiceDeps } from "@/hooks/use-voice-session";
import { sessionsApi, setStoredToken } from "@/services/api";
import { useAuthStore } from "@/store/auth-store";
import { MicError, type MicCapture, type MicCaptureOptions } from "@/services/voice/mic-capture";
import { PcmPlayer } from "@/services/voice/pcm-player";
import { decodeAudioFrame, isUuid } from "@/services/voice/protocol";
import { VoiceSocket } from "@/services/voice/voice-socket";
import {
  FakeAudioContext,
  FakeWebSocket,
  QUESTION_1,
  QUESTION_2,
  readyPayload,
} from "@/services/voice/__tests__/fakes";
import type { Session } from "@/types/exam-types";

const R1 = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
const R2 = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb";
const TTS_FORMAT = { encoding: "pcm_s16le", sampleRate: 24000, channels: 1 };

/** A microphone the test controls: it records how it was used and emits chunks on demand. */
class FakeMic {
  opts!: MicCaptureOptions;
  starts = 0;
  stops: boolean[] = [];
  active = false;
  startImpl: () => Promise<void> = async () => {};

  async start(): Promise<void> {
    this.starts++;
    await this.startImpl();
    this.active = true;
  }
  stop(flush = false): void {
    this.stops.push(flush);
    this.active = false;
  }
  isActive(): boolean {
    return this.active;
  }
  getSampleRate(): number {
    return 16000;
  }
  emit(samples = 1600): void {
    this.opts.onChunk(new Int16Array(samples));
  }
}

interface Setup {
  mic: FakeMic;
  clock: { now: number };
  props: {
    onSessionState: ReturnType<typeof vi.fn>;
    onAnswerSaved: ReturnType<typeof vi.fn>;
    onSessionEnded: ReturnType<typeof vi.fn>;
  };
  rerender(overrides: Options): void;
  unmount(): void;
}

interface Options {
  questionId?: string | null;
  disabled?: boolean;
  allowVoiceAnswer?: boolean;
}

function setup(initial: Options = {}): Setup {
  const mic = new FakeMic();
  const clock = { now: 1_000_000 };
  const deps: Partial<VoiceDeps> = {
    createSocket: (o) =>
      new VoiceSocket({
        ...o,
        WebSocketCtor: FakeWebSocket as unknown as typeof WebSocket,
        random: () => 0.5,
      }),
    createMic: (o) => {
      mic.opts = o;
      return mic as unknown as MicCapture;
    },
    createPlayer: (o) =>
      new PcmPlayer({ ...o, AudioContextCtor: FakeAudioContext as unknown as typeof AudioContext }),
    micSupported: () => true,
    now: () => clock.now,
  };
  const props = { onSessionState: vi.fn(), onAnswerSaved: vi.fn(), onSessionEnded: vi.fn() };
  const element = (o: Options) => (
    <VoiceAssistant
      sessionId="session-1"
      questionId={o.questionId === undefined ? QUESTION_1.id : o.questionId}
      disabled={o.disabled}
      allowVoiceAnswer={o.allowVoiceAnswer}
      deps={deps}
      {...props}
    />
  );
  const view = render(element(initial));
  return { mic, clock, props, rerender: (o) => view.rerender(element(o)), unmount: view.unmount };
}

const ws = () => FakeWebSocket.last;
const audio = () => FakeAudioContext.instances[0];
const flush = () => act(async () => {});

async function enable(overrides: Parameters<typeof readyPayload>[0] = {}): Promise<void> {
  fireEvent.click(screen.getByTestId("voice-enable-button"));
  await flush();
  act(() => ws().handshake(overrides));
}

/** Sends a typed command through the panel and returns its request id. */
function clickCommand(name: string): string {
  fireEvent.click(screen.getByRole("button", { name }));
  const commands = ws().ofType("command");
  return commands[commands.length - 1].id;
}

async function startRecording(): Promise<{ streamId: string; requestId: string }> {
  fireEvent.click(screen.getByRole("button", { name: "Start speaking" }));
  await flush();
  const starts = ws().ofType("audio.start");
  const start = starts[starts.length - 1];
  return { streamId: start.payload!.streamId as string, requestId: start.id };
}

function serverSpeaks(replyTo: string, responseId: string, text: string, chunks: number): void {
  act(() => {
    ws().serverSend("response", { responseId, kind: "question", text, audio: true }, replyTo);
    ws().serverSend("tts.start", { responseId, format: TTS_FORMAT }, replyTo);
    for (let seq = 0; seq < chunks; seq++) ws().serverAudio(responseId, seq, 2400);
  });
}

const stagedAnswer = {
  confirmationId: "conf-1",
  action: "select_option",
  questionId: QUESTION_1.id,
  optionKey: "B",
  optionText: "Paris",
  expiresAt: new Date(Date.now() + 60_000).toISOString(),
};

describe("VoiceAssistant", () => {
  beforeEach(() => {
    FakeWebSocket.reset();
    FakeAudioContext.reset();
    setStoredToken("access-token");
    useAuthStore.setState({ isAuthenticated: true });
    vi.spyOn(sessionsApi, "getSession").mockResolvedValue({
      id: "session-1",
      status: "IN_PROGRESS",
    } as Session);
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  describe("turning voice on", () => {
    it("opens no connection and touches no microphone until the student asks", () => {
      const { mic } = setup();
      expect(FakeWebSocket.instances).toHaveLength(0);
      expect(FakeAudioContext.instances).toHaveLength(0);
      expect(mic.starts).toBe(0);
      expect(
        screen.getByText(/microphone is used only while you are recording/i)
      ).toBeInTheDocument();
      expect(screen.getByTestId("voice-connection-status")).toHaveTextContent("Off");
    });

    it("connects with the token in a subprotocol and adopts the server's session state", async () => {
      const { mic, props } = setup();
      fireEvent.click(screen.getByTestId("voice-enable-button"));
      await flush();

      expect(FakeWebSocket.instances).toHaveLength(1);
      expect(ws().url).toBe("ws://localhost:8080/api/v1/sessions/session-1/voice");
      expect(ws().protocols).toEqual(["scribe.v1", "scribe.auth.access-token"]);
      expect(screen.getByTestId("voice-connection-status")).toHaveTextContent("Connecting");

      act(() => ws().handshake());
      expect(screen.getByTestId("voice-connection-status")).toHaveTextContent("Connected");
      expect(screen.getByTestId("voice-live-status")).toHaveTextContent(
        "Voice assistant connected."
      );
      expect(props.onSessionState).toHaveBeenCalledWith(
        expect.objectContaining({ status: "IN_PROGRESS" })
      );
      // Enabling voice is not consent to record.
      expect(mic.starts).toBe(0);
    });

    it("explains that speech input is unavailable when the server has no recognizer", async () => {
      setup();
      const ready = readyPayload();
      await enable({ capabilities: { ...ready.capabilities, stt: false } });

      expect(screen.getByRole("button", { name: "Start speaking" })).toBeDisabled();
      expect(
        screen.getByText(/speech recognition is not available on this server/i)
      ).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Read question" })).toBeEnabled();
    });

    it("releases everything when voice is turned off", async () => {
      setup();
      await enable();
      const socket = ws();
      fireEvent.click(screen.getByRole("button", { name: "Turn off voice" }));

      expect(socket.closedWith).toEqual({ code: 1000, reason: "client_stop" });
      expect(audio().close).toHaveBeenCalled();
      expect(screen.getByTestId("voice-connection-status")).toHaveTextContent("Off");
      expect(screen.getByTestId("voice-enable-button")).toBeInTheDocument();
    });
  });

  describe("command and answer modes", () => {
    it("offers dictation only for written-answer questions", async () => {
      setup({ allowVoiceAnswer: false });
      await enable();
      expect(screen.queryByTestId("voice-dictate-button")).toBeNull();
    });

    it("sends mode 'answer' only from the dictate control, and says so", async () => {
      setup({ allowVoiceAnswer: true });
      await enable();

      fireEvent.click(screen.getByTestId("voice-dictate-button"));
      await flush();
      const start = ws().ofType("audio.start").at(-1)!;
      expect(start.payload!.mode).toBe("answer");
      expect(screen.getByTestId("voice-activity")).toHaveTextContent("Listening to your answer");
    });

    it("sends mode 'command' from the ordinary speak control and the Alt+M shortcut", async () => {
      setup({ allowVoiceAnswer: true });
      await enable();

      await startRecording();
      expect(ws().ofType("audio.start").at(-1)!.payload!.mode).toBe("command");
      expect(screen.getByTestId("voice-activity")).toHaveTextContent("Listening for a command");
    });

    it("Alt+D dictates and Alt+M never does", async () => {
      setup({ allowVoiceAnswer: true });
      await enable();

      fireEvent.keyDown(window, { key: "d", altKey: true });
      await flush();
      expect(ws().ofType("audio.start").at(-1)!.payload!.mode).toBe("answer");
    });

    it("ignores Alt+D when the question takes no written answer", async () => {
      setup({ allowVoiceAnswer: false });
      await enable();

      fireEvent.keyDown(window, { key: "d", altKey: true });
      await flush();
      expect(ws().ofType("audio.start")).toHaveLength(0);
    });
  });

  describe("reading each new question aloud", () => {
    beforeEach(() => window.localStorage.clear());
    const repeats = () => ws().ofType("command").filter((m) => m.payload!.intent === "repeat_question");

    it("reads the question once when the page moves to it", async () => {
      const s = setup();
      await enable();
      expect(repeats()).toHaveLength(0); // not on connect: the first question is not read unasked

      s.rerender({ questionId: QUESTION_2.id });
      await flush();
      expect(repeats()).toHaveLength(1);

      // A re-render with the same question must not read it again.
      s.rerender({ questionId: QUESTION_2.id });
      s.rerender({ questionId: QUESTION_2.id, allowVoiceAnswer: true });
      await flush();
      expect(repeats()).toHaveLength(1);
    });

    it("does not read a question the backend already read during voice navigation", async () => {
      const s = setup();
      await enable();

      act(() => ws().serverSend("session.state", { status: "IN_PROGRESS", question: QUESTION_2 }));
      s.rerender({ questionId: QUESTION_2.id });
      await flush();
      expect(repeats()).toHaveLength(0);
    });

    it("waits until the page has finished navigating, then reads once", async () => {
      const s = setup();
      await enable();

      s.rerender({ questionId: QUESTION_2.id, disabled: true });
      await flush();
      expect(repeats()).toHaveLength(0);

      s.rerender({ questionId: QUESTION_2.id, disabled: false });
      await flush();
      expect(repeats()).toHaveLength(1);
    });

    it("stays silent when the student has switched it off, and remembers the choice", async () => {
      const s = setup();
      await enable();
      fireEvent.click(screen.getByTestId("voice-auto-read"));
      expect(window.localStorage.getItem("scribe.autoReadQuestions")).toBe("off");

      s.rerender({ questionId: QUESTION_2.id });
      await flush();
      expect(repeats()).toHaveLength(0);
    });

    it("does not read over a student who is already recording", async () => {
      const s = setup();
      await enable();
      await startRecording();

      s.rerender({ questionId: QUESTION_2.id });
      await flush();
      expect(repeats()).toHaveLength(0);
    });
  });

  describe("correcting a dictated answer before saving it", () => {
    const stagedText = {
      confirmationId: "conf-text",
      action: "dictate_answer",
      questionId: QUESTION_1.id,
      text: "plants make shugar",
      expiresAt: new Date(Date.now() + 60_000).toISOString(),
    };
    const stage = async () => {
      setup({ allowVoiceAnswer: true });
      await enable();
      act(() => ws().serverSend("confirm.request", stagedText));
    };
    const draft = () => screen.getByTestId("voice-answer-draft") as HTMLTextAreaElement;
    const lastCommand = () => ws().ofType("command").at(-1)!;

    it("shows what was heard in a labelled box the student can edit", async () => {
      await stage();
      expect(draft()).toHaveValue("plants make shugar");
      expect(screen.getByLabelText(/correct it here if it is wrong/i)).toBe(draft());
      expect(screen.getByTestId("voice-confirm-yes")).toHaveTextContent("Yes, save answer");
    });

    it("saves what was heard when the student changes nothing", async () => {
      await stage();
      fireEvent.click(screen.getByTestId("voice-confirm-yes"));

      expect(lastCommand().payload).toEqual({ intent: "confirm", confirmationId: "conf-text" });
    });

    it("sends the student's wording, with the confirmation it corrects", async () => {
      await stage();
      fireEvent.change(draft(), { target: { value: "plants make sugar" } });
      expect(screen.getByText(/you have changed the text/i)).toBeInTheDocument();

      const save = screen.getByTestId("voice-confirm-yes");
      expect(save).toHaveTextContent("Save my corrected answer");
      fireEvent.click(save);
      expect(lastCommand().payload).toEqual({
        intent: "confirm",
        confirmationId: "conf-text",
        text: "plants make sugar",
      });
    });

    it("treats a change back to the original as no change", async () => {
      await stage();
      fireEvent.change(draft(), { target: { value: "something else" } });
      fireEvent.change(draft(), { target: { value: "  plants make shugar " } });
      fireEvent.click(screen.getByTestId("voice-confirm-yes"));
      expect(lastCommand().payload).not.toHaveProperty("text");
    });

    it("will not save an empty answer", async () => {
      await stage();
      fireEvent.change(draft(), { target: { value: "   " } });

      expect(screen.getByTestId("voice-answer-empty")).toHaveTextContent(/cannot be empty/i);
      expect(draft()).toHaveAttribute("aria-invalid", "true");
      expect(screen.getByTestId("voice-confirm-yes")).toBeDisabled();
      expect(ws().ofType("command")).toHaveLength(0);
    });

    it("offers no editor for a chosen option", async () => {
      setup();
      await enable();
      act(() => ws().serverSend("confirm.request", stagedAnswer));
      expect(screen.queryByTestId("voice-answer-draft")).toBeNull();
      expect(screen.getByTestId("voice-confirmation")).toHaveTextContent("Option B");

      fireEvent.click(screen.getByTestId("voice-confirm-yes"));
      expect(lastCommand().payload).not.toHaveProperty("text");
    });

    it("keeps typing in the box from starting a recording", async () => {
      await stage();
      fireEvent.keyDown(draft(), { key: "Enter" });
      await flush();
      expect(ws().ofType("audio.start")).toHaveLength(0);
    });

    it("keeps the student's edit while a save is being retried", async () => {
      await stage();
      fireEvent.change(draft(), { target: { value: "plants make sugar" } });
      fireEvent.click(screen.getByTestId("voice-confirm-yes"));
      // The save could not be confirmed; the same answer is still staged.
      act(() =>
        ws().serverSend("action.result", {
          action: "answer",
          status: "failed",
          saved: false,
          retryable: true,
          confirmationId: "conf-text",
          questionId: QUESTION_1.id,
        })
      );
      expect(draft()).toHaveValue("plants make sugar");
    });

    it("says a corrected answer was saved, only once the server says so", async () => {
      await stage();
      fireEvent.change(draft(), { target: { value: "plants make sugar" } });
      fireEvent.click(screen.getByTestId("voice-confirm-yes"));
      expect(screen.getByTestId("voice-live-status")).not.toHaveTextContent(/saved/i);

      act(() =>
        ws().serverSend("action.result", {
          action: "answer",
          status: "ok",
          saved: true,
          corrected: true,
          confirmationId: "conf-text",
          questionId: QUESTION_1.id,
          answerId: "a1",
        })
      );
      expect(screen.getByTestId("voice-live-status")).toHaveTextContent("Corrected answer saved.");
      expect(screen.queryByTestId("voice-confirmation")).toBeNull();
    });
  });

  describe("answer audio retention", () => {
    const recordingCaps = {
      stt: true,
      tts: true,
      recording: true,
      ttsFormat: { encoding: "pcm_s16le", sampleRate: 24000, channels: 1 },
      intentClassifier: "rules",
    };

    it("says nothing about recording when the server does not keep audio", async () => {
      setup({ allowVoiceAnswer: true });
      await enable();
      expect(screen.queryByTestId("voice-recording-notice")).toBeNull();
    });

    it("tells the student audio is kept before they dictate, and that commands are not", async () => {
      setup({ allowVoiceAnswer: true });
      await enable({ capabilities: recordingCaps });
      const notice = screen.getByTestId("voice-recording-notice");
      expect(notice).toHaveTextContent(/recorded and kept/i);
      expect(notice).toHaveTextContent(/commands are not recorded/i);
    });

    it("reports whether the audio was kept, separately from whether the answer was", async () => {
      setup({ allowVoiceAnswer: true });
      await enable({ capabilities: recordingCaps });
      fireEvent.click(screen.getByTestId("voice-dictate-button"));
      await flush();
      const start = ws().ofType("audio.start").at(-1)!;

      act(() =>
        ws().serverSend(
          "recording.status",
          { streamId: start.payload!.streamId, status: "stored", recordingId: "r1", durationMs: 900 },
          start.id
        )
      );
      expect(screen.getByTestId("voice-recording-status")).toHaveTextContent(/audio .* was saved/i);
      expect(screen.queryByTestId("voice-answer-status")).toBeNull(); // the answer is not claimed saved

      act(() =>
        ws().serverSend(
          "recording.status",
          { streamId: start.payload!.streamId, status: "failed", code: "recording_failed" },
          start.id
        )
      );
      expect(screen.getByTestId("voice-recording-status")).toHaveTextContent(/could not be saved/i);
      expect(screen.getByTestId("voice-live-alert")).toHaveTextContent(/answer text is not affected/i);
    });

    it("ignores a status for an utterance that is no longer current", async () => {
      setup({ allowVoiceAnswer: true });
      await enable({ capabilities: recordingCaps });
      fireEvent.click(screen.getByTestId("voice-dictate-button"));
      await flush();

      act(() =>
        ws().serverSend("recording.status", { streamId: "some-older-stream", status: "failed" }, "old-request")
      );
      expect(screen.queryByTestId("voice-recording-status")).toBeNull();
    });
  });

  describe("Enter key", () => {
    // The toggle lock reads Date.now, so each test controls the clock.
    let now = 0;
    beforeEach(() => {
      now = Date.now(); // start at the real time so expiry timestamps still make sense
      vi.spyOn(Date, "now").mockImplementation(() => now);
    });
    afterEach(() => vi.restoreAllMocks());
    const tick = (ms: number) => {
      now += ms;
    };

    it("starts a command on a multiple-choice question and stops on the next press", async () => {
      const { mic } = setup({ allowVoiceAnswer: false });
      await enable();

      fireEvent.keyDown(document.body, { key: "Enter" });
      await flush();
      expect(ws().ofType("audio.start").at(-1)!.payload!.mode).toBe("command");
      expect(screen.getByTestId("voice-recording-indicator")).toBeInTheDocument();

      tick(1000);
      act(() => mic.emit());
      fireEvent.keyDown(document.body, { key: "Enter" });
      await flush();
      expect(screen.queryByTestId("voice-recording-indicator")).not.toBeInTheDocument();
      expect(ws().ofType("audio.end")).toHaveLength(1);
    });

    it("dictates an answer on a written-answer question", async () => {
      setup({ allowVoiceAnswer: true });
      await enable();

      fireEvent.keyDown(document.body, { key: "Enter" });
      await flush();
      expect(ws().ofType("audio.start").at(-1)!.payload!.mode).toBe("answer");
    });

    it("leaves Enter to buttons, text fields and dialogs", async () => {
      setup({ allowVoiceAnswer: true });
      await enable();
      render(
        <div>
          <textarea aria-label="essay" />
          <button type="button">Next</button>
          <div role="dialog" aria-label="d">
            <span>in dialog</span>
          </div>
        </div>
      );

      fireEvent.keyDown(screen.getByLabelText("essay"), { key: "Enter" });
      fireEvent.keyDown(screen.getByRole("button", { name: "Next" }), { key: "Enter" });
      fireEvent.keyDown(screen.getByText("in dialog"), { key: "Enter" });
      await flush();
      expect(ws().ofType("audio.start")).toHaveLength(0);
    });

    it("does nothing for a held key, a modified Enter, or while composing text", async () => {
      setup({ allowVoiceAnswer: true });
      await enable();

      fireEvent.keyDown(document.body, { key: "Enter", repeat: true });
      fireEvent.keyDown(document.body, { key: "Enter", shiftKey: true });
      fireEvent.keyDown(document.body, { key: "Enter", ctrlKey: true });
      fireEvent.keyDown(document.body, { key: "Enter", isComposing: true });
      await flush();
      expect(ws().ofType("audio.start")).toHaveLength(0);
    });

    it("treats two presses in quick succession as one", async () => {
      setup({ allowVoiceAnswer: true });
      await enable();

      fireEvent.keyDown(document.body, { key: "Enter" });
      tick(50);
      fireEvent.keyDown(document.body, { key: "Enter" });
      await flush();
      expect(ws().ofType("audio.start")).toHaveLength(1);
      expect(ws().ofType("audio.end")).toHaveLength(0);
      expect(screen.getByTestId("voice-recording-indicator")).toBeInTheDocument();
    });

    it("does not record while an answer waits for confirmation, and says why", async () => {
      setup({ allowVoiceAnswer: true });
      await enable();
      act(() => ws().serverSend("confirm.request", stagedAnswer));

      fireEvent.keyDown(document.body, { key: "Enter" });
      await flush();
      expect(ws().ofType("audio.start")).toHaveLength(0);
      expect(screen.getByTestId("voice-live-alert")).toHaveTextContent(/answer waiting/i);
    });

    it("never confirms an answer, even with the confirmation on screen", async () => {
      setup({ allowVoiceAnswer: true });
      await enable();
      act(() => ws().serverSend("confirm.request", stagedAnswer));

      fireEvent.keyDown(document.body, { key: "Enter" });
      await flush();
      expect(ws().ofType("command")).toHaveLength(0);
    });
  });

  describe("recording and streaming audio", () => {
    it("asks for the microphone only on Start speaking, then streams ordered PCM frames", async () => {
      const { mic } = setup();
      await enable();
      const { streamId } = await startRecording();

      expect(mic.starts).toBe(1);
      expect(isUuid(streamId)).toBe(true);
      expect(ws().ofType("audio.start")[0].payload).toEqual({
        streamId,
        encoding: "pcm_s16le",
        sampleRate: 16000,
        channels: 1,
        mode: "command",
      });
      expect(screen.getByTestId("voice-recording-indicator")).toHaveTextContent("Recording");
      expect(screen.getByTestId("voice-mic-button")).toHaveAttribute("aria-pressed", "true");

      act(() => {
        mic.emit();
        mic.emit();
        mic.emit(800);
      });
      const frames = ws()
        .binary()
        .map((b) => decodeAudioFrame(b)!);
      expect(frames.map((f) => f.seq)).toEqual([0, 1, 2]);
      expect(frames.every((f) => f.id === streamId)).toBe(true);
      expect(frames.map((f) => f.audio.length)).toEqual([3200, 3200, 1600]);

      fireEvent.click(screen.getByRole("button", { name: "Stop and send" }));
      expect(mic.stops).toEqual([true]);
      expect(ws().ofType("audio.end")[0].payload).toEqual({ streamId, chunks: 3 });
      expect(screen.queryByTestId("voice-recording-indicator")).not.toBeInTheDocument();
      expect(screen.getByTestId("voice-activity")).toHaveTextContent("Processing");
    });

    it("discards a recording on request with audio.cancel", async () => {
      const { mic } = setup();
      await enable();
      const { streamId } = await startRecording();
      act(() => mic.emit());

      fireEvent.click(screen.getByRole("button", { name: "Discard recording" }));

      expect(ws().ofType("audio.cancel")[0].payload).toEqual({ streamId });
      expect(ws().ofType("audio.end")).toHaveLength(0);
      expect(mic.stops).toEqual([false]);
      expect(screen.getByTestId("voice-activity")).toHaveTextContent("Ready");
    });

    it("cancels instead of sending an empty stream when nothing was captured", async () => {
      setup();
      await enable();
      const { streamId } = await startRecording();
      fireEvent.click(screen.getByRole("button", { name: "Stop and send" }));

      expect(ws().ofType("audio.end")).toHaveLength(0);
      expect(ws().ofType("audio.cancel")[0].payload).toEqual({ streamId });
      expect(screen.getByTestId("voice-live-alert")).toHaveTextContent(/no audio was captured/i);
    });

    it("explains how to recover when microphone permission is denied, and sends nothing", async () => {
      const { mic } = setup();
      mic.startImpl = async () => {
        throw new MicError("denied");
      };
      await enable();
      fireEvent.click(screen.getByRole("button", { name: "Start speaking" }));
      await flush();

      expect(ws().ofType("audio.start")).toHaveLength(0);
      expect(screen.getByTestId("voice-mic-help")).toHaveTextContent(/site settings/i);
      expect(screen.getByTestId("voice-live-alert")).toHaveTextContent(
        /microphone access was blocked/i
      );
      expect(screen.queryByTestId("voice-recording-indicator")).not.toBeInTheDocument();
      // The rest of the assistant still works.
      expect(screen.getByRole("button", { name: "Read question" })).toBeEnabled();
      expect(screen.getByRole("button", { name: "Start speaking" })).toBeEnabled();
    });

    it("ignores a second Start speaking while the permission prompt is open", async () => {
      const { mic } = setup();
      let grant: () => void = () => {};
      mic.startImpl = () => new Promise<void>((r) => (grant = r));
      await enable();

      fireEvent.click(screen.getByRole("button", { name: "Start speaking" }));
      await flush();
      expect(screen.getByTestId("voice-activity")).toHaveTextContent(
        /waiting for microphone permission/i
      );
      expect(screen.getByTestId("voice-mic-button")).toBeDisabled();
      fireEvent.keyDown(window, { key: "m", altKey: true });
      await flush();
      expect(mic.starts).toBe(1);

      await act(async () => grant());
      expect(ws().ofType("audio.start")).toHaveLength(1);
    });

    it("abandons the utterance when the network cannot keep up, instead of buffering audio", async () => {
      const { mic } = setup();
      await enable();
      const { streamId } = await startRecording();
      act(() => mic.emit());
      ws().bufferedAmount = 10 * 1024 * 1024;
      act(() => mic.emit());

      expect(ws().binary()).toHaveLength(1);
      expect(ws().ofType("audio.cancel")[0].payload).toEqual({ streamId });
      expect(mic.stops).toEqual([false]);
      expect(screen.getByTestId("voice-live-alert")).toHaveTextContent(/too slow/i);
    });

    it("ends the stream by itself before the server's duration limit", async () => {
      const { mic, clock } = setup();
      await enable();
      const { streamId } = await startRecording();
      act(() => mic.emit());
      clock.now += 44_500;
      act(() => mic.emit());

      expect(ws().ofType("audio.end")[0].payload).toEqual({ streamId, chunks: 2 });
      expect(screen.queryByTestId("voice-recording-indicator")).not.toBeInTheDocument();
      act(() => mic.emit());
      expect(ws().binary()).toHaveLength(2);
    });

    it("stops recording when the server fails the stream", async () => {
      const { mic } = setup();
      await enable();
      const { streamId, requestId } = await startRecording();
      act(() => mic.emit());
      act(() =>
        ws().serverSend(
          "error",
          { code: "chunk_out_of_order", message: "x", retryable: false, streamId },
          requestId
        )
      );

      expect(mic.stops).toEqual([false]);
      expect(screen.queryByTestId("voice-recording-indicator")).not.toBeInTheDocument();
      expect(screen.getByTestId("voice-live-alert")).toHaveTextContent(/audio was lost/i);
      act(() => mic.emit());
      expect(ws().binary()).toHaveLength(1);
    });
  });

  describe("transcripts, intent and confirmation", () => {
    async function sayOptionB(s: Setup): Promise<string> {
      const { streamId, requestId } = await startRecording();
      act(() => s.mic.emit());
      fireEvent.click(screen.getByRole("button", { name: "Stop and send" }));
      act(() => {
        ws().serverSend("stt.partial", { streamId, text: "option" }, requestId);
      });
      expect(screen.getByTestId("voice-transcript")).toHaveTextContent("option");
      act(() => {
        ws().serverSend("stt.final", { streamId, text: "option B" }, requestId);
        ws().serverSend(
          "intent",
          { streamId, intent: "select_option", optionKey: "B", confidence: 1, classifier: "rules" },
          requestId
        );
        ws().serverSend("confirm.request", stagedAnswer, requestId);
        ws().serverSend(
          "response",
          {
            responseId: R1,
            streamId,
            kind: "confirmation_request",
            text: "You chose option B: Paris. Say yes to save this answer, or no to cancel.",
            audio: false,
          },
          requestId
        );
      });
      return requestId;
    }

    it("shows the transcript and the staged answer as not yet saved", async () => {
      const s = setup();
      await enable();
      await sayOptionB(s);

      expect(screen.getByTestId("voice-transcript")).toHaveTextContent("You said: option B");
      const confirmation = screen.getByRole("group", { name: "Confirm your answer" });
      expect(within(confirmation).getByText("Option B: Paris")).toBeInTheDocument();
      expect(within(confirmation).getByText(/not saved yet/i)).toBeInTheDocument();
      expect(screen.getByTestId("voice-answer-status")).toHaveTextContent(
        /waiting for your confirmation/i
      );
      expect(s.props.onAnswerSaved).not.toHaveBeenCalled();
      // The frontend sent no answer of its own: only audio went to the server.
      expect(ws().ofType("command")).toHaveLength(0);
    });

    it("saves only after the student confirms and the backend reports persistence", async () => {
      const s = setup();
      await enable();
      await sayOptionB(s);

      fireEvent.click(screen.getByTestId("voice-confirm-yes"));
      const confirm = ws().ofType("command")[0];
      expect(confirm.payload).toEqual({ intent: "confirm", confirmationId: "conf-1" });
      expect(screen.getByTestId("voice-answer-status")).toHaveTextContent(/saving/i);
      expect(s.props.onAnswerSaved).not.toHaveBeenCalled();

      // A second click while the save is in flight sends nothing more.
      fireEvent.click(screen.getByTestId("voice-confirm-yes"));
      expect(ws().ofType("command")).toHaveLength(1);

      act(() => {
        ws().serverSend(
          "action.result",
          {
            action: "answer",
            status: "ok",
            saved: true,
            confirmationId: "conf-1",
            questionId: QUESTION_1.id,
            optionKey: "B",
            answerId: "ans-1",
          },
          confirm.id
        );
      });
      expect(screen.getByTestId("voice-answer-status")).toHaveTextContent(
        /answer saved\. confirmed by the exam server/i
      );
      expect(screen.queryByTestId("voice-confirmation")).not.toBeInTheDocument();
      expect(s.props.onAnswerSaved).toHaveBeenCalledTimes(1);
      expect(s.props.onAnswerSaved).toHaveBeenCalledWith(QUESTION_1.id);
      expect(screen.getByTestId("voice-live-status")).toHaveTextContent("Answer saved.");
    });

    it("reports a persistence failure as not saved and lets the student retry", async () => {
      const s = setup();
      await enable();
      await sayOptionB(s);
      fireEvent.click(screen.getByTestId("voice-confirm-yes"));
      const first = ws().ofType("command")[0];

      act(() => {
        ws().serverSend(
          "action.result",
          {
            action: "answer",
            status: "failed",
            saved: false,
            code: "service_unavailable",
            retryable: true,
            confirmationId: "conf-1",
            questionId: QUESTION_1.id,
          },
          first.id
        );
      });
      expect(screen.getByTestId("voice-answer-status")).toHaveTextContent(
        /could not be confirmed as saved/i
      );
      expect(s.props.onAnswerSaved).not.toHaveBeenCalled();
      expect(screen.getByTestId("voice-live-alert")).toHaveTextContent(/could not be confirmed/i);

      // The retry is the student's explicit action, never automatic.
      expect(ws().ofType("command")).toHaveLength(1);
      fireEvent.click(screen.getByTestId("voice-confirm-yes"));
      expect(ws().ofType("command")[1].payload).toEqual({
        intent: "confirm",
        confirmationId: "conf-1",
      });
    });

    it("cancels a staged answer with the No button", async () => {
      const s = setup();
      await enable();
      await sayOptionB(s);
      fireEvent.click(screen.getByTestId("voice-confirm-no"));
      const deny = ws().ofType("command")[0];
      expect(deny.payload).toEqual({ intent: "deny", confirmationId: "conf-1" });

      act(() => {
        ws().serverSend(
          "action.result",
          { action: "answer", status: "cancelled", saved: false, confirmationId: "conf-1" },
          deny.id
        );
      });
      expect(screen.queryByTestId("voice-confirmation")).not.toBeInTheDocument();
      expect(screen.getByTestId("voice-answer-status")).toHaveTextContent("Answer not saved.");
      expect(s.props.onAnswerSaved).not.toHaveBeenCalled();
    });

    it("applies a duplicate saved result once", async () => {
      const s = setup();
      await enable();
      await sayOptionB(s);
      fireEvent.click(screen.getByTestId("voice-confirm-yes"));
      const id = ws().ofType("command")[0].id;
      const result = {
        action: "answer",
        status: "ok",
        saved: true,
        confirmationId: "conf-1",
        questionId: QUESTION_1.id,
      };

      act(() => {
        ws().serverSend("action.result", result, id);
        ws().serverSend("action.result", { ...result, duplicate: true }, id);
      });
      expect(screen.getByTestId("voice-answer-status")).toHaveTextContent(/answer saved/i);
      // Re-reading the answer twice is harmless: it is a read, not a mutation.
      expect(s.props.onAnswerSaved).toHaveBeenCalledWith(QUESTION_1.id);
    });

    it("stops offering a staged answer when the confirmation expires", async () => {
      vi.useFakeTimers();
      const s = setup();
      fireEvent.click(screen.getByTestId("voice-enable-button"));
      await act(async () => {});
      act(() => ws().handshake());
      act(() => {
        ws().serverSend("confirm.request", {
          ...stagedAnswer,
          expiresAt: new Date(Date.now() + 5000).toISOString(),
        });
      });
      expect(screen.getByTestId("voice-confirmation")).toBeInTheDocument();

      act(() => {
        vi.advanceTimersByTime(5001);
      });
      expect(screen.queryByTestId("voice-confirmation")).not.toBeInTheDocument();
      expect(screen.getByTestId("voice-live-status")).toHaveTextContent(/timed out.*not saved/i);
      expect(ws().ofType("command")).toHaveLength(0);
      s.unmount();
    });

    it("hides a staged answer once the page shows a different question", async () => {
      const s = setup();
      await enable();
      await sayOptionB(s);
      expect(screen.getByTestId("voice-confirmation")).toBeInTheDocument();

      s.rerender({ questionId: QUESTION_2.id });
      expect(screen.queryByTestId("voice-confirmation")).not.toBeInTheDocument();
    });

    it("announces a clarification and changes nothing", async () => {
      const s = setup();
      await enable();
      const { streamId, requestId } = await startRecording();
      act(() => s.mic.emit());
      fireEvent.click(screen.getByRole("button", { name: "Stop and send" }));
      act(() => {
        ws().serverSend("stt.final", { streamId, text: "um the second one maybe" }, requestId);
        ws().serverSend(
          "intent",
          { streamId, intent: "unknown", confidence: 0, classifier: "rules" },
          requestId
        );
        ws().serverSend(
          "response",
          {
            responseId: R1,
            streamId,
            kind: "clarification",
            code: "not_understood",
            text: "Sorry, I didn't understand.",
            audio: false,
          },
          requestId
        );
      });

      expect(screen.getByTestId("voice-response")).toHaveTextContent("Sorry, I didn't understand.");
      expect(screen.getByTestId("voice-live-status")).toHaveTextContent(
        "Sorry, I didn't understand."
      );
      expect(screen.queryByTestId("voice-confirmation")).not.toBeInTheDocument();
      expect(screen.queryByTestId("voice-answer-status")).not.toBeInTheDocument();
      expect(ws().ofType("command")).toHaveLength(0);
      expect(s.props.onAnswerSaved).not.toHaveBeenCalled();
    });
  });

  describe("speech playback and barge-in", () => {
    it("plays the response to the current request in order", async () => {
      setup();
      await enable();
      const id = clickCommand("Read question");
      expect(ws().ofType("command")[0].payload).toEqual({ intent: "repeat_question" });

      serverSpeaks(id, R1, "Question 1 of 2. What is the capital of France?", 3);

      expect(audio().sources).toHaveLength(3);
      const starts = audio().sources.map((s) => s.startedAt!);
      expect(starts[0]).toBeLessThan(starts[1]);
      expect(starts[1]).toBeLessThan(starts[2]);
      expect(screen.getByTestId("voice-response")).toHaveTextContent(
        "What is the capital of France?"
      );
      expect(screen.getByTestId("voice-activity")).toHaveTextContent("Speaking");
      // Spoken responses are not also pushed to the screen reader.
      expect(screen.getByTestId("voice-live-status")).not.toHaveTextContent("capital of France");

      act(() => {
        ws().serverSend("tts.end", { responseId: R1, reason: "completed", chunks: 3 }, id);
        audio().sources.forEach((s) => s.finish());
      });
      expect(screen.getByTestId("voice-activity")).toHaveTextContent("Ready");
      expect(screen.getByRole("button", { name: "Replay response" })).toBeInTheDocument();
    });

    it("stops speech and tells the server when the student starts speaking (barge-in)", async () => {
      const { mic } = setup();
      await enable();
      const id = clickCommand("Read question");
      serverSpeaks(id, R1, "Question 1 of 2.", 2);
      expect(audio().sources.every((s) => !s.stopped)).toBe(true);

      const { streamId, requestId } = await startRecording();

      // Local half: playback stopped at once. Server half: audio.start is the
      // protocol's barge-in signal.
      expect(audio().sources.every((s) => s.stopped)).toBe(true);
      expect(ws().ofType("audio.start")).toHaveLength(1);
      expect(screen.getByTestId("voice-activity")).toHaveTextContent("Listening");

      // Chunks of the interrupted response that were already on the wire.
      act(() => {
        ws().serverAudio(R1, 2, 2400);
        ws().serverAudio(R1, 3, 2400);
        ws().serverSend("tts.end", { responseId: R1, reason: "barge_in", chunks: 4 }, id);
      });
      expect(audio().sources).toHaveLength(2);

      // The answer to the new utterance plays normally.
      act(() => mic.emit());
      fireEvent.click(screen.getByRole("button", { name: "Stop and send" }));
      act(() => ws().serverSend("stt.final", { streamId, text: "next question" }, requestId));
      serverSpeaks(requestId, R2, "Question 2 of 2.", 1);
      expect(audio().sources).toHaveLength(3);
      expect(audio().sources[2].stopped).toBe(false);
    });

    it("never shows or plays the response to a request that was superseded", async () => {
      setup();
      await enable();
      const first = clickCommand("Read question");
      const second = clickCommand("Time left");

      serverSpeaks(first, R1, "Question 1 of 2.", 2);
      expect(audio().sources).toHaveLength(0);
      expect(screen.queryByTestId("voice-response")).not.toBeInTheDocument();

      serverSpeaks(second, R2, "You have 50 minutes left.", 1);
      expect(audio().sources).toHaveLength(1);
      expect(screen.getByTestId("voice-response")).toHaveTextContent("50 minutes");
    });

    it("ignores audio that no response announced", async () => {
      setup();
      await enable();
      act(() => {
        ws().serverAudio(R1, 0, 2400);
        ws().serverSend("tts.start", { responseId: R1, format: TTS_FORMAT });
        ws().serverAudio(R1, 0, 2400);
      });
      expect(audio().sources).toHaveLength(0);
    });

    it("stops speech from the button and from Alt+S, sending tts.cancel", async () => {
      setup();
      await enable();
      const id = clickCommand("Read question");
      serverSpeaks(id, R1, "Question 1 of 2.", 2);

      fireEvent.click(screen.getByRole("button", { name: "Stop speaking" }));
      expect(audio().sources.every((s) => s.stopped)).toBe(true);
      expect(ws().ofType("tts.cancel")[0].payload).toEqual({ responseId: R1 });
      expect(screen.queryByRole("button", { name: "Stop speaking" })).not.toBeInTheDocument();
      act(() => ws().serverAudio(R1, 2, 2400));
      expect(audio().sources).toHaveLength(2);

      fireEvent.keyDown(window, { key: "s", altKey: true });
      expect(ws().ofType("tts.cancel")).toHaveLength(2);
    });

    it("stops speech about the old question when the page navigates by itself", async () => {
      const s = setup();
      await enable();
      const id = clickCommand("Read question");
      serverSpeaks(id, R1, "Question 1 of 2.", 2);

      // The student pressed the on-screen Next button (HTTP navigation).
      s.rerender({ questionId: QUESTION_2.id });

      expect(audio().sources.every((src) => src.stopped)).toBe(true);
      expect(ws().ofType("tts.cancel")[0].payload).toEqual({ responseId: R1 });
    });

    it("keeps speaking when the question changed because of voice navigation", async () => {
      const s = setup();
      await enable();
      const id = clickCommand("Read question");
      act(() => {
        ws().serverSend(
          "action.result",
          { action: "navigate", status: "ok", saved: false, direction: "next" },
          id
        );
        ws().serverSend("session.state", { status: "IN_PROGRESS", question: QUESTION_2 }, id);
      });
      expect(s.props.onSessionState).toHaveBeenLastCalledWith(
        expect.objectContaining({ question: expect.objectContaining({ id: QUESTION_2.id }) })
      );
      serverSpeaks(id, R1, "Question 2 of 2.", 2);

      s.rerender({ questionId: QUESTION_2.id });

      expect(audio().sources.every((src) => !src.stopped)).toBe(true);
      expect(ws().ofType("tts.cancel")).toHaveLength(0);
    });

    it("shows the text and offers a play button when the browser blocks audio", async () => {
      FakeAudioContext.initialState = "suspended";
      FakeAudioContext.resumable = false;
      setup();
      await enable();
      const id = clickCommand("Read question");
      serverSpeaks(id, R1, "Question 1 of 2.", 2);

      expect(screen.getByTestId("voice-response")).toHaveTextContent("Question 1 of 2.");
      expect(screen.getByTestId("voice-live-alert")).toHaveTextContent(/audio is blocked/i);
      expect(audio().sources).toHaveLength(0);

      FakeAudioContext.resumable = true;
      fireEvent.click(screen.getByRole("button", { name: "Play response" }));
      await flush();
      expect(audio().sources).toHaveLength(2);
    });

    it("announces the text when the server's audio fails", async () => {
      setup();
      await enable();
      const id = clickCommand("Read question");
      act(() => {
        ws().serverSend(
          "response",
          { responseId: R1, kind: "question", text: "Question 1 of 2.", audio: true },
          id
        );
        ws().serverSend(
          "tts.end",
          { responseId: R1, reason: "error", code: "tts_failed", chunks: 0 },
          id
        );
      });
      expect(screen.getByTestId("voice-live-status")).toHaveTextContent("Question 1 of 2.");
    });
  });

  describe("session end, reconnects and cleanup", () => {
    it("stops recording and playback and tells the page when the session ends", async () => {
      const { mic, props } = setup();
      await enable();
      const id = clickCommand("Read question");
      serverSpeaks(id, R1, "Question 1 of 2.", 2);
      await startRecording();
      act(() => mic.emit());

      act(() => {
        ws().serverSend("error", { code: "session_expired", message: "x", retryable: false });
        ws().serverSend("session.closed", { reason: "expired" });
        ws().serverClose(4410);
      });

      expect(mic.active).toBe(false);
      expect(audio().sources.every((s) => s.stopped)).toBe(true);
      expect(props.onSessionEnded).toHaveBeenCalledTimes(1);
      expect(screen.getByTestId("voice-connection-status")).toHaveTextContent("Stopped");
      expect(screen.getByTestId("voice-live-alert")).toHaveTextContent(/session has ended/i);
      expect(screen.getByRole("button", { name: "Start speaking" })).toBeDisabled();
      // No reconnect, and nothing more is sent.
      expect(FakeWebSocket.instances).toHaveLength(1);
      act(() => mic.emit());
      expect(FakeWebSocket.last.binary()).toHaveLength(1);
    });

    it("releases the socket, microphone and audio when unmounted mid-recording (submission)", async () => {
      const s = setup();
      await enable();
      const socket = ws();
      const id = clickCommand("Read question");
      serverSpeaks(id, R1, "Question 1 of 2.", 2);
      await startRecording();
      act(() => s.mic.emit());
      const sentBefore = socket.sent.length;

      s.unmount();

      expect(s.mic.active).toBe(false);
      expect(socket.closedWith).toEqual({ code: 1000, reason: "client_stop" });
      expect(socket.onmessage).toBeNull();
      expect(audio().sources.every((src) => src.stopped)).toBe(true);
      expect(audio().close).toHaveBeenCalled();
      // Late device and network events are inert.
      s.mic.emit();
      expect(socket.sent).toHaveLength(sentBefore);
      expect(FakeWebSocket.instances).toHaveLength(1);
    });

    it("pauses voice input while the page is navigating or submitting", async () => {
      const s = setup();
      await enable();
      const { streamId } = await startRecording();
      act(() => s.mic.emit());

      s.rerender({ disabled: true });

      expect(s.mic.active).toBe(false);
      expect(ws().ofType("audio.cancel")[0].payload).toEqual({ streamId });
      expect(screen.getByRole("button", { name: "Start speaking" })).toBeDisabled();
      expect(screen.getByRole("button", { name: "Read question" })).toBeDisabled();
      fireEvent.keyDown(window, { key: "m", altKey: true });
      await flush();
      expect(ws().ofType("audio.start")).toHaveLength(1);
    });

    it("stops recording on a network drop, reconnects, and starts clean", async () => {
      vi.useFakeTimers();
      const s = setup();
      fireEvent.click(screen.getByTestId("voice-enable-button"));
      await act(async () => {});
      act(() => ws().handshake());
      act(() => ws().serverSend("confirm.request", stagedAnswer));
      fireEvent.click(screen.getByRole("button", { name: "Start speaking" }));
      await act(async () => {});
      act(() => s.mic.emit());

      act(() => ws().serverClose(1006));
      expect(s.mic.active).toBe(false);
      expect(screen.getByTestId("voice-connection-status")).toHaveTextContent(
        "Reconnecting (attempt 1)"
      );
      expect(screen.getByTestId("voice-live-alert")).toHaveTextContent(/reconnecting/i);
      expect(screen.queryByTestId("voice-confirmation")).not.toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Start speaking" })).toBeDisabled();

      await act(async () => {
        await vi.advanceTimersByTimeAsync(1000);
      });
      expect(FakeWebSocket.instances).toHaveLength(2);
      act(() =>
        ws().handshake({ resumed: true, session: { status: "IN_PROGRESS", question: QUESTION_2 } })
      );

      expect(screen.getByTestId("voice-connection-status")).toHaveTextContent("Connected");
      expect(screen.getByTestId("voice-live-status")).toHaveTextContent(
        "Voice assistant reconnected."
      );
      expect(s.props.onSessionState).toHaveBeenLastCalledWith(
        expect.objectContaining({ question: expect.objectContaining({ id: QUESTION_2.id }) })
      );
      expect(screen.queryByTestId("voice-confirmation")).not.toBeInTheDocument();
      s.unmount();
      expect(vi.getTimerCount()).toBe(0);
    });

    it("offers a manual reconnect, not a retry loop, when another tab takes over", async () => {
      const { props } = setup();
      await enable();
      act(() => ws().serverClose(4409));

      expect(screen.getByText(/active in another tab or device/i)).toBeInTheDocument();
      expect(props.onSessionEnded).not.toHaveBeenCalled();
      expect(FakeWebSocket.instances).toHaveLength(1);

      fireEvent.click(screen.getByRole("button", { name: "Reconnect" }));
      expect(FakeWebSocket.instances).toHaveLength(2);
      act(() => ws().handshake());
      expect(screen.getByTestId("voice-connection-status")).toHaveTextContent("Connected");
    });

    it("disconnects immediately on logout", async () => {
      setup();
      await enable();
      const socket = ws();
      act(() => useAuthStore.setState({ isAuthenticated: false }));

      expect(socket.closedWith).toEqual({ code: 1000, reason: "client_stop" });
      expect(screen.getByTestId("voice-connection-status")).toHaveTextContent("Off");
    });

    it("stops when the upgrade is refused because the session is over", async () => {
      vi.spyOn(sessionsApi, "getSession").mockResolvedValue({
        id: "session-1",
        status: "SUBMITTED",
      } as Session);
      const { props } = setup();
      fireEvent.click(screen.getByTestId("voice-enable-button"));
      await flush();
      await act(async () => ws().serverClose(1006));
      await flush();

      expect(props.onSessionEnded).toHaveBeenCalledTimes(1);
      expect(screen.getByTestId("voice-connection-status")).toHaveTextContent("Stopped");
      expect(FakeWebSocket.instances).toHaveLength(1);
    });
  });

  describe("accessibility", () => {
    it("exposes labelled controls that work from the keyboard", async () => {
      const { mic } = setup();
      await enable();

      for (const name of [
        "Start speaking",
        "Read question",
        "Read my answer",
        "Time left",
        "Turn off voice",
      ]) {
        const button = screen.getByRole("button", { name });
        expect(button.tagName).toBe("BUTTON");
        expect(button).not.toHaveAttribute("tabindex", "-1");
      }
      expect(screen.getByRole("group", { name: "Voice controls" })).toBeInTheDocument();
      expect(screen.getByText(/alt\+m starts or stops a voice command/i)).toBeInTheDocument();

      // Alt+M toggles recording without touching the mouse.
      fireEvent.keyDown(window, { key: "m", altKey: true });
      await flush();
      expect(mic.starts).toBe(1);
      expect(screen.getByRole("button", { name: "Stop and send" })).toHaveAttribute(
        "aria-pressed",
        "true"
      );
      act(() => mic.emit());
      fireEvent.keyDown(window, { key: "M", altKey: true });
      expect(ws().ofType("audio.end")).toHaveLength(1);
    });

    it("does not react to shortcuts until voice is on", async () => {
      const { mic } = setup();
      fireEvent.keyDown(window, { key: "m", altKey: true });
      await flush();
      expect(mic.starts).toBe(0);
      expect(FakeWebSocket.instances).toHaveLength(0);
    });

    it("announces status through a polite region and errors through an alert", async () => {
      setup();
      await enable();
      const status = screen.getByTestId("voice-live-status");
      const alert = screen.getByTestId("voice-live-alert");
      expect(status).toHaveAttribute("role", "status");
      expect(status).toHaveAttribute("aria-live", "polite");
      expect(alert).toHaveAttribute("role", "alert");
      expect(status).toHaveTextContent("Voice assistant connected.");
      expect(alert).toBeEmptyDOMElement();
    });

    it("does not announce interim transcripts or audio chunks", async () => {
      const { mic } = setup();
      await enable();
      const { streamId, requestId } = await startRecording();
      act(() => mic.emit());
      const statusBefore = screen.getByTestId("voice-live-status").textContent;

      act(() => {
        for (const text of ["o", "op", "opt", "opti", "option"]) {
          ws().serverSend("stt.partial", { streamId, text }, requestId);
        }
      });
      expect(screen.getByTestId("voice-transcript")).toHaveAttribute("aria-live", "off");
      expect(screen.getByTestId("voice-transcript")).toHaveTextContent("option");
      expect(screen.getByTestId("voice-live-status").textContent).toBe(statusBefore);
      expect(screen.getByTestId("voice-live-alert")).toBeEmptyDOMElement();

      fireEvent.click(screen.getByRole("button", { name: "Stop and send" }));
      serverSpeaks(requestId, R1, "Question 1 of 2.", 5);
      expect(screen.getByTestId("voice-live-status").textContent).toBe(statusBefore);
    });
  });
});
