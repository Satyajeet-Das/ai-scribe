"use client";

import { useCallback, useEffect, useMemo, useReducer, useRef } from "react";
import { API_BASE_URL, ApiError, getStoredToken, sessionsApi } from "@/services/api";
import { useAuthStore } from "@/store/auth-store";
import {
  MicCapture,
  MicError,
  isMicSupported,
  type MicErrorKind,
} from "@/services/voice/mic-capture";
import { PcmPlayer } from "@/services/voice/pcm-player";
import {
  PCM_ENCODING,
  newUuid,
  type AudioMode,
  type CommandIntent,
  type ServerMessage,
  type VoiceSessionState,
} from "@/services/voice/protocol";
import {
  VoiceSocket,
  voiceSocketUrl,
  type ProbeResult,
  type VoiceSocketOptions,
} from "@/services/voice/voice-socket";
import {
  initialVoiceState,
  voiceReducer,
  type MicState,
  type VoiceState,
} from "@/services/voice/voice-state";

/** Length of each audio chunk sent to the server. */
const CHUNK_MILLIS = 100;
/** If this much audio is waiting in the browser's send buffer, the network cannot keep up. */
const MAX_BUFFERED_BYTES = 256 * 1024;
/** Recording stops this long before the server's limit so the last chunk still fits. */
const STREAM_MARGIN_MILLIS = 750;

/** Collaborators that tests replace. */
export interface VoiceDeps {
  createSocket(options: VoiceSocketOptions): VoiceSocket;
  createMic(options: ConstructorParameters<typeof MicCapture>[0]): MicCapture;
  createPlayer(options: ConstructorParameters<typeof PcmPlayer>[0]): PcmPlayer;
  micSupported(): boolean;
  now(): number;
}

const defaultDeps: VoiceDeps = {
  createSocket: (options) => new VoiceSocket(options),
  createMic: (options) => new MicCapture(options),
  createPlayer: (options) => new PcmPlayer(options),
  micSupported: isMicSupported,
  now: () => Date.now(),
};

export interface UseVoiceSessionOptions {
  sessionId: string;
  /** The question currently shown, as the page knows it. */
  questionId: string | null;
  /** True while the page is navigating or submitting; voice input is paused. */
  disabled?: boolean;
  /** The backend reported the session's state (on connect, sync and navigation). */
  onSessionState?(state: VoiceSessionState): void;
  /** The backend confirmed that an answer was persisted for this question. */
  onAnswerSaved?(questionId: string): void;
  /** The session is over, or access to it was lost. */
  onSessionEnded?(): void;
  /**
   * Read each newly shown question aloud when the page moves to it. It happens
   * once per move, and never on the first question, a reconnection or a
   * re-render.
   */
  autoReadQuestions?: boolean;
  deps?: Partial<VoiceDeps>;
}

export interface VoiceController {
  state: VoiceState;
  /** The staged answer, only while it still applies to the question on screen. */
  pending: VoiceState["pending"];
  canReplay: boolean;
  enable(): void;
  disable(): void;
  reconnect(): void;
  /**
   * Starts recording. Triggers the microphone permission prompt the first time.
   * "command" (the default) listens for a spoken request; "answer" dictates an
   * answer, whose words are never treated as commands.
   */
  startSpeaking(mode?: AudioMode): Promise<void>;
  /** Stops recording and sends the utterance for recognition. */
  finishSpeaking(): void;
  /** Stops recording and discards the utterance. */
  cancelSpeaking(): void;
  sendCommand(intent: CommandIntent, optionKey?: string): void;
  /**
   * Confirms the staged answer. For a dictated answer, `text` is the student's
   * corrected wording; omit it to save what was heard.
   */
  confirm(text?: string): void;
  deny(): void;
  stopSpeech(): void;
  replaySpeech(): void;
  /** Shows and announces a message to the student. */
  announce(text: string, tone?: "info" | "error"): void;
}

async function probeSession(sessionId: string): Promise<ProbeResult> {
  try {
    // apiFetch refreshes an expired access token on its own, so a successful
    // probe also means the next connection attempt has a valid token.
    const session = await sessionsApi.getSession(sessionId);
    return session.status === "IN_PROGRESS" ? "ok" : "ended";
  } catch (err) {
    if (err instanceof ApiError) {
      if (err.status === 401) return "unauthorized";
      if (err.status === 403) return "forbidden";
      if (err.status === 404) return "ended";
    }
    return "unreachable";
  }
}

const MIC_STATE: Record<MicErrorKind, MicState> = {
  denied: "denied",
  unsupported: "unsupported",
  no_device: "no_device",
  failed: "error",
};

const MIC_TEXT: Record<MicErrorKind, string> = {
  denied:
    "Microphone access was blocked. Allow the microphone in your browser's site settings, then try again.",
  unsupported: "This browser cannot record audio. Use the buttons and keyboard instead.",
  no_device: "No microphone was found. Connect one and try again.",
  failed: "The microphone stopped working. Check it and try again.",
};

export function useVoiceSession(options: UseVoiceSessionOptions): VoiceController {
  const { sessionId, questionId, disabled = false } = options;
  const [state, dispatch] = useReducer(voiceReducer, initialVoiceState);

  // Latest values for callbacks that outlive a render.
  const latest = useRef({ options, state });
  useEffect(() => {
    latest.current = { options, state };
  });

  const socketRef = useRef<VoiceSocket | null>(null);
  const micRef = useRef<MicCapture | null>(null);
  const playerRef = useRef<PcmPlayer | null>(null);

  /** The audio stream being recorded. */
  const captureRef = useRef<{
    streamId: string;
    seq: number;
    bytes: number;
    startedAt: number;
  } | null>(null);
  /** Guards against overlapping startSpeaking calls while the permission prompt is open. */
  const startingRef = useRef(false);
  /** The request whose response and speech are accepted; mirrors the reducer. */
  const activeRequestRef = useRef<string | null>(null);
  const acceptedResponseRef = useRef<string | null>(null);
  /** The question the backend last reported over this connection. */
  const voiceQuestionRef = useRef<string | null>(null);
  /** The question that should be read as soon as it is possible to do so. */
  const autoReadPendingRef = useRef<string | null>(null);

  const deps = useMemo<VoiceDeps>(() => ({ ...defaultDeps, ...options.deps }), [options.deps]);

  // -------------------------------------------------------------------------
  // Teardown helpers
  // -------------------------------------------------------------------------

  const stopCapture = useCallback((notice?: string) => {
    startingRef.current = false;
    const capture = captureRef.current;
    captureRef.current = null;
    micRef.current?.stop(false);
    if (capture) {
      socketRef.current?.send("audio.cancel", { streamId: capture.streamId });
      activeRequestRef.current = null;
      dispatch({ type: "capture_aborted", notice });
    }
    dispatch({ type: "mic", state: "idle" });
  }, []);

  const silence = useCallback(() => {
    acceptedResponseRef.current = null;
    playerRef.current?.stop();
  }, []);

  const teardown = useCallback(() => {
    startingRef.current = false;
    captureRef.current = null;
    activeRequestRef.current = null;
    acceptedResponseRef.current = null;
    voiceQuestionRef.current = null;
    micRef.current?.stop(false);
    micRef.current = null;
    playerRef.current?.dispose();
    playerRef.current = null;
    socketRef.current?.dispose();
    socketRef.current = null;
  }, []);

  // -------------------------------------------------------------------------
  // Incoming
  // -------------------------------------------------------------------------

  const handleMessage = useCallback(
    (message: ServerMessage) => {
      const { options: opts } = latest.current;

      switch (message.type) {
        case "ready":
          // A fresh connection: nothing from the previous one is still valid.
          captureRef.current = null;
          micRef.current?.stop(false);
          activeRequestRef.current = null;
          silence();
          dispatch({ type: "mic", state: "idle" });
          voiceQuestionRef.current = message.payload.session.question?.id ?? null;
          opts.onSessionState?.(message.payload.session);
          break;

        case "session.state":
          voiceQuestionRef.current = message.payload.question?.id ?? voiceQuestionRef.current;
          opts.onSessionState?.(message.payload);
          break;

        case "action.result":
          if (
            message.payload.action === "answer" &&
            message.payload.saved &&
            message.payload.questionId
          ) {
            opts.onAnswerSaved?.(message.payload.questionId);
          }
          break;

        case "response":
          // Only the response to the request in progress is spoken or shown.
          if (message.replyTo !== null && message.replyTo === activeRequestRef.current) {
            acceptedResponseRef.current = message.payload.audio ? message.payload.responseId : null;
          }
          break;

        case "tts.start":
          if (message.payload.responseId === acceptedResponseRef.current) {
            playerRef.current?.begin(message.payload.responseId, message.payload.format);
          }
          break;

        case "tts.end":
          playerRef.current?.end(message.payload.responseId);
          break;

        case "error":
          if (captureRef.current && message.payload.streamId === captureRef.current.streamId) {
            // The server failed the stream; stop recording into the void.
            captureRef.current = null;
            micRef.current?.stop(false);
            dispatch({ type: "mic", state: "idle" });
          }
          break;

        default:
          break;
      }
      dispatch({ type: "server", message });
    },
    [silence]
  );

  // -------------------------------------------------------------------------
  // Controls
  // -------------------------------------------------------------------------

  const enable = useCallback(() => {
    if (socketRef.current) return;

    const player = deps.createPlayer({
      onState: (speech) => dispatch({ type: "speech", state: speech }),
    });
    playerRef.current = player;
    // This runs inside the click that enabled voice, which is what lets the
    // browser start audio output later without another gesture.
    void player.unlock();

    const socket = deps.createSocket({
      url: voiceSocketUrl(API_BASE_URL, sessionId),
      getToken: getStoredToken,
      refreshToken: () => useAuthStore.getState().refreshToken(),
      probe: () => probeSession(sessionId),
      handlers: {
        onStatus: (status, detail) => {
          if (status !== "connected") {
            // Recording or speech on a dead connection would go nowhere.
            captureRef.current = null;
            startingRef.current = false;
            activeRequestRef.current = null;
            micRef.current?.stop(false);
            acceptedResponseRef.current = null;
            playerRef.current?.stop();
            dispatch({ type: "mic", state: "idle" });
          }
          dispatch({ type: "connection", status, ...detail });
          if (
            status === "error" &&
            detail.fatal &&
            detail.fatal !== "replaced" &&
            detail.fatal !== "protocol"
          ) {
            latest.current.options.onSessionEnded?.();
          }
        },
        onMessage: handleMessage,
        onAudio: (frame) => {
          // The player rejects frames for any response other than the accepted one.
          playerRef.current?.push(frame.id, frame.seq, frame.audio);
        },
      },
    });
    socketRef.current = socket;
    dispatch({ type: "enabled" });
    socket.start();
  }, [deps, handleMessage, sessionId]);

  const disable = useCallback(() => {
    teardown();
    dispatch({ type: "disabled" });
  }, [teardown]);

  const reconnect = useCallback(() => {
    socketRef.current?.retry();
  }, []);

  const finishSpeaking = useCallback(() => {
    const capture = captureRef.current;
    if (!capture) return;
    // Flushing delivers the last partial chunk through onChunk before the end.
    micRef.current?.stop(true);
    // The flush can itself end or abort the stream (size limit, lost connection).
    if (captureRef.current !== capture) return;
    captureRef.current = null;
    dispatch({ type: "mic", state: "idle" });

    const socket = socketRef.current;
    if (capture.seq === 0) {
      socket?.send("audio.cancel", { streamId: capture.streamId });
      activeRequestRef.current = null;
      dispatch({ type: "capture_aborted", notice: "No audio was captured. Please try again." });
      return;
    }
    if (socket?.send("audio.end", { streamId: capture.streamId, chunks: capture.seq }) == null) {
      activeRequestRef.current = null;
      dispatch({
        type: "capture_aborted",
        notice: "The voice connection was lost. Please try again.",
      });
      return;
    }
    dispatch({ type: "capture_ended" });
  }, []);

  const handleChunk = useCallback(
    (pcm: Int16Array) => {
      const capture = captureRef.current;
      const socket = socketRef.current;
      const limits = latest.current.state.limits;
      if (!capture || !socket || !limits) return;

      // Bounded buffering: audio is never queued in page memory, and if the
      // browser's own send buffer backs up, the utterance is abandoned.
      if (socket.bufferedAmount() > MAX_BUFFERED_BYTES) {
        stopCapture("Your connection is too slow to send audio. Please try again.");
        return;
      }
      const bytes = pcm.length * 2;
      if (capture.bytes + bytes > limits.maxStreamBytes) {
        finishSpeaking();
        return;
      }
      if (!socket.sendAudio(capture.streamId, capture.seq, pcm)) {
        stopCapture("The voice connection was lost. Please try again.");
        return;
      }
      capture.seq++;
      capture.bytes += bytes;

      if (deps.now() - capture.startedAt >= limits.maxStreamMillis - STREAM_MARGIN_MILLIS) {
        dispatch({ type: "notice", tone: "info", text: "Maximum recording length reached." });
        finishSpeaking();
      }
    },
    [deps, finishSpeaking, stopCapture]
  );

  const startSpeaking = useCallback(async (mode: AudioMode = "command") => {
    const { state: current, options: opts } = latest.current;
    const socket = socketRef.current;
    if (!socket || !socket.isConnected() || opts.disabled) return;
    if (captureRef.current || startingRef.current) return;
    if (!current.capabilities?.stt || !current.limits) return;
    if (!current.limits.audioEncodings.includes(PCM_ENCODING)) return;

    if (!deps.micSupported()) {
      dispatch({ type: "mic", state: "unsupported" });
      dispatch({ type: "notice", tone: "error", text: MIC_TEXT.unsupported });
      return;
    }

    // Barge-in, local half: stop speech now. The server half is audio.start.
    silence();

    let mic = micRef.current;
    if (!mic) {
      const limits = current.limits;
      mic = deps.createMic({
        allowedSampleRates: limits.audioSampleRates,
        chunkMillis: CHUNK_MILLIS,
        onChunk: handleChunk,
        onInterrupted: (kind) => {
          if (captureRef.current) {
            socketRef.current?.send("audio.cancel", { streamId: captureRef.current.streamId });
            captureRef.current = null;
            activeRequestRef.current = null;
            dispatch({ type: "capture_aborted", notice: MIC_TEXT[kind] });
          }
          dispatch({ type: "mic", state: MIC_STATE[kind] });
        },
      });
      micRef.current = mic;
    }

    startingRef.current = true;
    dispatch({ type: "mic", state: "requesting" });
    try {
      await mic.start();
    } catch (err) {
      startingRef.current = false;
      const kind: MicErrorKind = err instanceof MicError ? err.kind : "failed";
      // A capture torn down while the prompt was open is not an error to report.
      if (micRef.current !== mic || socketRef.current !== socket) return;
      dispatch({ type: "mic", state: MIC_STATE[kind] });
      dispatch({ type: "notice", tone: "error", text: MIC_TEXT[kind] });
      return;
    }
    if (!startingRef.current || socketRef.current !== socket || !socket.isConnected()) {
      // Voice was turned off, or the connection dropped, while permission was pending.
      startingRef.current = false;
      mic.stop(false);
      dispatch({ type: "mic", state: "idle" });
      return;
    }
    startingRef.current = false;

    const streamId = newUuid();
    const requestId = socket.send("audio.start", {
      streamId,
      encoding: PCM_ENCODING,
      sampleRate: mic.getSampleRate(),
      channels: 1,
      mode,
    });
    if (requestId === null) {
      mic.stop(false);
      dispatch({ type: "mic", state: "idle" });
      return;
    }
    captureRef.current = { streamId, seq: 0, bytes: 0, startedAt: deps.now() };
    activeRequestRef.current = requestId;
    acceptedResponseRef.current = null;
    dispatch({ type: "mic", state: "recording" });
    dispatch({ type: "request_started", id: requestId, kind: "audio", streamId, mode });
  }, [deps, handleChunk, silence]);

  const cancelSpeaking = useCallback(() => {
    if (captureRef.current || startingRef.current) stopCapture();
  }, [stopCapture]);

  const sendCommand = useCallback(
    (intent: CommandIntent, optionKey?: string, confirmationId?: string, text?: string) => {
      const socket = socketRef.current;
      if (!socket || latest.current.options.disabled) return;
      // A command supersedes whatever is in progress, here and on the server.
      if (captureRef.current || startingRef.current) stopCapture();
      silence();

      const requestId = socket.send("command", {
        intent,
        ...(optionKey ? { optionKey } : {}),
        ...(confirmationId ? { confirmationId } : {}),
        ...(text !== undefined ? { text } : {}),
      });
      if (requestId === null) {
        dispatch({ type: "notice", tone: "error", text: "The voice assistant is not connected." });
        return;
      }
      activeRequestRef.current = requestId;
      dispatch({ type: "request_started", id: requestId, kind: "command", intent });
    },
    [silence, stopCapture]
  );

  const pendingRaw = state.pending;
  // A staged answer belongs to one question. If the page has moved on (for
  // example through the on-screen Next button), it is not offered any more;
  // the backend voids it on its side as well.
  const pending =
    pendingRaw && (questionId === null || pendingRaw.questionId === questionId) ? pendingRaw : null;

  const confirm = useCallback(
    (text?: string) => {
      const p = latest.current.state.pending;
      // A correction only means something for a dictated answer.
      if (p) sendCommand("confirm", undefined, p.confirmationId, p.optionKey ? undefined : text);
    },
    [sendCommand]
  );

  const deny = useCallback(() => {
    const p = latest.current.state.pending;
    if (p) sendCommand("deny", undefined, p.confirmationId);
  }, [sendCommand]);

  const stopSpeech = useCallback(() => {
    const responseId = acceptedResponseRef.current ?? playerRef.current?.getActiveId() ?? undefined;
    silence();
    socketRef.current?.send("tts.cancel", responseId ? { responseId } : {});
  }, [silence]);

  const replaySpeech = useCallback(() => {
    void playerRef.current?.replay();
  }, []);

  const announce = useCallback((text: string, tone: "info" | "error" = "info") => {
    dispatch({ type: "notice", tone, text });
  }, []);

  // -------------------------------------------------------------------------
  // Lifecycle
  // -------------------------------------------------------------------------

  // Release everything on unmount (submission, expiry, navigation away).
  useEffect(() => teardown, [teardown]);

  // Logging out ends the voice connection immediately.
  useEffect(() => {
    return useAuthStore.subscribe((auth) => {
      if (!auth.isAuthenticated && socketRef.current) {
        teardown();
        dispatch({ type: "disabled" });
      }
    });
  }, [teardown]);

  // While the page is navigating or submitting, nothing is recorded.
  useEffect(() => {
    if (disabled && (captureRef.current || startingRef.current)) stopCapture();
  }, [disabled, stopCapture]);

  // The question changed without the backend telling us over this connection,
  // i.e. through the page's own HTTP navigation. Speech about the old question
  // must not keep playing over the new one.
  useEffect(() => {
    if (questionId === null || questionId === voiceQuestionRef.current) return;
    voiceQuestionRef.current = questionId;
    if (latest.current.options.autoReadQuestions) autoReadPendingRef.current = questionId;
    if (playerRef.current && playerRef.current.getState() !== "idle") {
      const responseId = acceptedResponseRef.current ?? undefined;
      silence();
      socketRef.current?.send("tts.cancel", responseId ? { responseId } : {});
    }
  }, [questionId, silence]);

  // Auto-read. The request is held until reading is possible (the page has
  // finished navigating, the connection is up and speech is available) and is
  // dropped if the connection is lost or the student starts speaking first, so
  // a reconnection or a late recovery never reads a question out of the blue.
  const connection = state.connection;
  const ttsAvailable = state.capabilities?.tts === true;
  useEffect(() => {
    const wanted = autoReadPendingRef.current;
    if (!wanted) return;
    if (wanted !== questionId || !options.autoReadQuestions || connection !== "connected") {
      autoReadPendingRef.current = null;
      return;
    }
    if (captureRef.current || startingRef.current) {
      autoReadPendingRef.current = null;
      return;
    }
    if (disabled || !ttsAvailable) return;
    autoReadPendingRef.current = null;
    sendCommand("repeat_question");
  }, [questionId, disabled, connection, ttsAvailable, options.autoReadQuestions, sendCommand]);

  // A staged answer expires on the backend; stop offering it at the same moment.
  const pendingId = pendingRaw?.confirmationId;
  const pendingExpiry = pendingRaw?.expiresAt;
  useEffect(() => {
    if (!pendingId || !pendingExpiry) return;
    const remaining = Date.parse(pendingExpiry) - Date.now();
    if (Number.isNaN(remaining)) return;
    const timer = setTimeout(
      () => dispatch({ type: "confirmation_expired", confirmationId: pendingId }),
      Math.max(0, remaining)
    );
    return () => clearTimeout(timer);
  }, [pendingId, pendingExpiry]);

  return {
    state,
    pending,
    // The last response is kept by the player until the next one starts.
    canReplay: state.response?.audio === true && state.speech !== "playing",
    enable,
    disable,
    reconnect,
    startSpeaking,
    finishSpeaking,
    cancelSpeaking,
    sendCommand,
    confirm,
    deny,
    stopSpeech,
    replaySpeech,
    announce,
  };
}
