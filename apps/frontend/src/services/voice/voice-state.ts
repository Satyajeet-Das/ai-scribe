/**
 * UI state for the voice assistant, as a pure reducer over protocol events.
 *
 * The reducer only records what the backend reported. It never decides what
 * an utterance meant, whether an answer is valid, or that something was saved:
 * "saved" appears only after an `action.result` with `saved: true`.
 */
import type {
  ActionResultPayload,
  ConfirmRequestPayload,
  ReadyPayload,
  ResponsePayload,
  ServerMessage,
  VoiceLimits,
} from "./protocol";
import type { ConnectionStatus, FatalReason } from "./voice-socket";
import type { PlaybackState } from "./pcm-player";
import type { AudioMode } from "./protocol";

export type MicState =
  | "idle"
  | "requesting"
  | "recording"
  | "denied"
  | "unsupported"
  | "no_device"
  | "error";
export type BusyState = "idle" | "listening" | "processing";

export type AnswerPhase =
  | "idle"
  | "awaiting_confirmation"
  | "saving"
  | "saved"
  | "failed"
  | "cancelled";

export interface AnswerStatus {
  phase: AnswerPhase;
  questionId?: string;
  optionKey?: string;
  code?: string;
  retryable?: boolean;
}

/** Whether the audio of the last dictated answer was kept. Independent of the answer. */
export type RecordingPhase = "none" | "stored" | "failed";

export interface Notice {
  /** Increases with every notice so identical text is still re-announced. */
  seq: number;
  tone: "info" | "error";
  text: string;
}

export interface VoiceState {
  enabled: boolean;
  connection: ConnectionStatus;
  attempt: number;
  fatal: FatalReason | null;
  sessionReason: string | null;
  capabilities: ReadyPayload["capabilities"] | null;
  limits: VoiceLimits | null;

  mic: MicState;
  busy: BusyState;
  /** The request whose transcript and response are currently accepted. */
  activeRequestId: string | null;
  activeStreamId: string | null;
  /** What the recording in progress is for. Only meaningful while recording. */
  captureMode: AudioMode | null;

  partialTranscript: string;
  finalTranscript: string;
  intent: { name: string; optionKey?: string } | null;

  /** An answer the backend has staged and is waiting for a yes or no on. Not saved. */
  pending: ConfirmRequestPayload | null;
  answer: AnswerStatus;
  recording: RecordingPhase;

  response: ResponsePayload | null;
  speech: PlaybackState;
  notice: Notice | null;
}

export const initialVoiceState: VoiceState = {
  enabled: false,
  connection: "idle",
  attempt: 0,
  fatal: null,
  sessionReason: null,
  capabilities: null,
  limits: null,
  mic: "idle",
  busy: "idle",
  activeRequestId: null,
  activeStreamId: null,
  captureMode: null,
  partialTranscript: "",
  finalTranscript: "",
  intent: null,
  pending: null,
  answer: { phase: "idle" },
  recording: "none",
  response: null,
  speech: "idle",
  notice: null,
};

export type VoiceAction =
  | { type: "enabled" }
  | { type: "disabled" }
  | {
      type: "connection";
      status: ConnectionStatus;
      attempt?: number;
      fatal?: FatalReason;
      sessionReason?: string;
    }
  | { type: "server"; message: ServerMessage }
  | {
      type: "request_started";
      id: string;
      kind: "audio" | "command";
      streamId?: string;
      intent?: string;
      mode?: AudioMode;
    }
  | { type: "capture_ended" }
  | { type: "capture_aborted"; notice?: string }
  | { type: "mic"; state: MicState }
  | { type: "speech"; state: PlaybackState }
  | { type: "confirmation_expired"; confirmationId: string }
  | { type: "notice"; tone: "info" | "error"; text: string };

function notify(state: VoiceState, tone: Notice["tone"], text: string): Notice {
  return { seq: (state.notice?.seq ?? 0) + 1, tone, text };
}

const FATAL_TEXT: Record<FatalReason, string> = {
  auth: "Voice assistant stopped: your login has expired. Please sign in again.",
  forbidden: "Voice assistant stopped: you do not have access to this exam session.",
  session_ended: "Voice assistant stopped: this exam session has ended.",
  replaced: "Voice assistant stopped: this exam was opened in another tab or device.",
  protocol: "Voice assistant stopped because of a communication problem.",
};

/** Student-facing text for backend error codes. Presentation only. */
const ERROR_TEXT: Record<string, string> = {
  stt_unavailable: "Speech recognition is not available. Use the buttons and keyboard instead.",
  stt_failed: "Speech recognition failed. Please try again.",
  stt_timeout: "Speech recognition took too long. Please try again.",
  provider_busy: "The voice service is busy. Please try again in a moment.",
  audio_backpressure: "Your audio could not be processed in time. Please try again.",
  stream_too_long: "That recording was too long. Please try again with a shorter answer.",
  stream_too_large: "That recording was too long. Please try again with a shorter answer.",
  empty_stream: "No audio was received. Please try again.",
  chunk_count_mismatch: "Some audio was lost on the way. Please try again.",
  chunk_out_of_order: "Some audio was lost on the way. Please try again.",
  invalid_audio_format: "This microphone's audio format is not supported.",
  stream_already_active: "A recording is already in progress.",
  intent_unavailable: "The request could not be understood right now. Nothing was changed.",
  session_expired: "The exam time has expired.",
  session_submitted: "The exam has already been submitted.",
  session_inactive: "The exam session is no longer active.",
  exam_unavailable: "The exam is currently unavailable.",
  busy: "The exam system is busy. Nothing was changed. Please try again.",
  service_unavailable: "The exam service is temporarily unavailable. Nothing was changed.",
  rate_limited: "Too many requests. Please wait a moment.",
  forbidden: "You do not have access to this exam session.",
};

/** Codes that are bookkeeping, not something to tell the student about. */
const SILENT_CODES = new Set(["duplicate_request", "unknown_stream", "turn_cancelled"]);

function errorText(code: string): string {
  return ERROR_TEXT[code] ?? "Something went wrong with the voice assistant. Please try again.";
}

function applyActionResult(state: VoiceState, result: ActionResultPayload): VoiceState {
  if (result.action !== "answer") {
    // Navigation: the new position arrives in session.state and the response
    // text explains a refusal. A failure is worth a notice of its own.
    if (result.status === "failed" && result.code) {
      return { ...state, notice: notify(state, "error", errorText(result.code)) };
    }
    return state;
  }

  const base: AnswerStatus = {
    phase: "idle",
    questionId: result.questionId,
    optionKey: result.optionKey,
  };
  switch (result.status) {
    case "ok":
      // Only the backend's confirmation of persistence counts as saved.
      if (!result.saved) return state;
      return {
        ...state,
        pending: null,
        answer: { ...base, phase: "saved" },
        notice: notify(
          state,
          "info",
          result.corrected ? "Corrected answer saved." : "Answer saved."
        ),
      };
    case "cancelled":
      return {
        ...state,
        pending: null,
        answer: { ...base, phase: "cancelled" },
        notice: notify(
          state,
          "info",
          result.code === "superseded"
            ? "Your earlier answer was replaced by the new one and was not saved."
            : "Answer not saved."
        ),
      };
    case "failed":
    case "rejected":
    default:
      return {
        ...state,
        // A retryable failure leaves the confirmation open on the backend.
        pending: result.retryable ? state.pending : null,
        answer: { ...base, phase: "failed", code: result.code, retryable: result.retryable },
        notice: notify(
          state,
          "error",
          result.retryable
            ? "Your answer could not be confirmed as saved. You can try again."
            : "Your answer was not saved."
        ),
      };
  }
}

function applyServer(state: VoiceState, message: ServerMessage): VoiceState {
  const forActive = message.replyTo !== null && message.replyTo === state.activeRequestId;

  switch (message.type) {
    case "ready":
      // A new connection starts clean: the backend keeps no dialog state
      // across connections, so neither does the client.
      return {
        ...state,
        capabilities: message.payload.capabilities,
        limits: message.payload.limits,
        busy: "idle",
        activeRequestId: null,
        activeStreamId: null,
        partialTranscript: "",
        pending: null,
        recording: "none",
        answer: state.answer.phase === "saved" ? state.answer : { phase: "idle" },
        notice: notify(
          state,
          "info",
          message.payload.resumed ? "Voice assistant reconnected." : "Voice assistant connected."
        ),
      };

    case "stt.partial":
      if (!forActive) return state;
      return { ...state, partialTranscript: message.payload.text };

    case "stt.final":
      if (!forActive) return state;
      return {
        ...state,
        partialTranscript: "",
        finalTranscript: message.payload.text,
        busy: "processing",
      };

    case "intent": {
      if (!forActive) return state;
      const next: VoiceState = {
        ...state,
        intent: { name: message.payload.intent, optionKey: message.payload.optionKey },
      };
      if (message.payload.intent === "confirm" && state.pending) {
        next.answer = {
          phase: "saving",
          questionId: state.pending.questionId,
          optionKey: state.pending.optionKey,
        };
      }
      return next;
    }

    // The next two describe durable or dialog state on the backend. The
    // connection delivers them in the order the backend acted, so they are
    // applied whichever request they answer.
    case "confirm.request":
      return {
        ...state,
        pending: message.payload,
        answer: {
          phase: "awaiting_confirmation",
          questionId: message.payload.questionId,
          optionKey: message.payload.optionKey,
        },
      };

    case "action.result":
      return applyActionResult(state, message.payload);

    case "session.state": {
      const questionId = message.payload.question?.id;
      if (state.pending && questionId && state.pending.questionId !== questionId) {
        return { ...state, pending: null, answer: { phase: "idle" } };
      }
      return state;
    }

    case "response": {
      if (!forActive) return state;
      const next: VoiceState = { ...state, response: message.payload, busy: "idle" };
      // With audio the student hears the response; without it, it is announced.
      if (!message.payload.audio) {
        next.notice = notify(
          state,
          message.payload.kind === "error" ? "error" : "info",
          message.payload.text
        );
      }
      return next;
    }

    case "recording.status":
      // Only the utterance in progress; a late report for an earlier one is stale.
      if (!forActive && message.payload.streamId !== state.activeStreamId) return state;
      if (message.payload.status === "stored") return { ...state, recording: "stored" };
      return {
        ...state,
        recording: "failed",
        notice: notify(
          state,
          "error",
          "The audio of your answer could not be kept. Your answer text is not affected."
        ),
      };

    case "tts.end":
      if (
        message.payload.reason === "error" &&
        message.payload.responseId === state.response?.responseId
      ) {
        // The audio failed; fall back to announcing the text that was shown.
        return { ...state, notice: notify(state, "info", state.response.text) };
      }
      return state;

    case "error": {
      const { code, streamId } = message.payload;
      const mine = forActive || (!!streamId && streamId === state.activeStreamId);
      const sessionLevel =
        code.startsWith("session_") || code === "forbidden" || code === "rate_limited";
      if (!mine && !sessionLevel) return state;

      const next: VoiceState = mine
        ? {
            ...state,
            busy: "idle",
            partialTranscript: "",
            activeRequestId: null,
            activeStreamId: null,
          }
        : state;
      if (mine && state.answer.phase === "saving") {
        next.answer = {
          ...state.answer,
          phase: "failed",
          code,
          retryable: message.payload.retryable,
        };
      }
      if (SILENT_CODES.has(code)) return next;
      return { ...next, notice: notify(state, "error", errorText(code)) };
    }

    case "session.closed":
    case "auth.expiring":
    case "tts.start":
    case "ack":
    case "pong":
    default:
      return state;
  }
}

export function voiceReducer(state: VoiceState, action: VoiceAction): VoiceState {
  switch (action.type) {
    case "enabled":
      return { ...initialVoiceState, enabled: true, connection: "connecting" };

    case "disabled":
      return { ...initialVoiceState, notice: notify(state, "info", "Voice assistant turned off.") };

    case "connection": {
      if (!state.enabled) return state;
      const next: VoiceState = {
        ...state,
        connection: action.status,
        attempt: action.attempt ?? 0,
        fatal: action.fatal ?? null,
        sessionReason: action.sessionReason ?? null,
      };
      if (action.status === "connected") return next;

      // Anything in flight on the old connection is void.
      next.busy = "idle";
      next.activeRequestId = null;
      next.activeStreamId = null;
      next.partialTranscript = "";
      next.pending = null;
      if (state.answer.phase === "saving" || state.answer.phase === "awaiting_confirmation") {
        next.answer = { ...state.answer, phase: "failed", retryable: true };
      }
      if (action.status === "reconnecting" && state.connection === "connected") {
        next.notice = notify(state, "error", "Voice connection lost. Reconnecting.");
      } else if (action.status === "disconnected") {
        next.notice = notify(
          state,
          "error",
          "Voice connection lost. Use the reconnect button to try again."
        );
      } else if (action.status === "error" && action.fatal) {
        next.notice = notify(state, "error", FATAL_TEXT[action.fatal]);
      }
      return next;
    }

    case "server":
      return state.enabled ? applyServer(state, action.message) : state;

    case "request_started": {
      const next: VoiceState = {
        ...state,
        activeRequestId: action.id,
        activeStreamId: action.streamId ?? null,
        captureMode: action.kind === "audio" ? (action.mode ?? "command") : null,
        recording: action.kind === "audio" ? "none" : state.recording,
        busy: action.kind === "audio" ? "listening" : "processing",
        partialTranscript: "",
        finalTranscript: "",
        intent: null,
        response: null,
      };
      if (action.intent === "confirm" && state.pending) {
        next.answer = {
          phase: "saving",
          questionId: state.pending.questionId,
          optionKey: state.pending.optionKey,
        };
      }
      return next;
    }

    case "capture_ended":
      return state.busy === "listening" ? { ...state, busy: "processing" } : state;

    case "capture_aborted":
      return {
        ...state,
        busy: "idle",
        activeRequestId: null,
        activeStreamId: null,
        partialTranscript: "",
        notice: action.notice ? notify(state, "error", action.notice) : state.notice,
      };

    case "mic":
      return { ...state, mic: action.state };

    case "speech":
      if (action.state === "blocked" && state.speech !== "blocked") {
        return {
          ...state,
          speech: action.state,
          notice: notify(
            state,
            "error",
            "Audio is blocked by the browser. Use the Play response button."
          ),
        };
      }
      return { ...state, speech: action.state };

    case "confirmation_expired":
      if (state.pending?.confirmationId !== action.confirmationId) return state;
      return {
        ...state,
        pending: null,
        answer: { ...state.answer, phase: "cancelled" },
        notice: notify(state, "info", "The confirmation timed out. Your answer was not saved."),
      };

    case "notice":
      return { ...state, notice: notify(state, action.tone, action.text) };

    default:
      return state;
  }
}
