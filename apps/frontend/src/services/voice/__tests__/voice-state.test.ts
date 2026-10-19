import { describe, expect, it } from "vitest";
import type { ServerMessage, ServerMessageType, ServerPayload } from "../protocol";
import { initialVoiceState, voiceReducer, type VoiceAction, type VoiceState } from "../voice-state";
import { QUESTION_1, QUESTION_2, readyPayload } from "./fakes";

function server<T extends ServerMessageType>(
  type: T,
  payload: ServerPayload<T>,
  replyTo: string | null = null
): VoiceAction {
  return { type: "server", message: { type, replyTo, payload } as ServerMessage };
}

function run(state: VoiceState, ...actions: VoiceAction[]): VoiceState {
  return actions.reduce(voiceReducer, state);
}

const connected = run(
  initialVoiceState,
  { type: "enabled" },
  { type: "connection", status: "connected" },
  server("ready", readyPayload())
);

const confirmRequest = {
  confirmationId: "conf-1",
  action: "select_option",
  questionId: QUESTION_1.id,
  optionKey: "B",
  optionText: "Paris",
  expiresAt: "2099-01-01T00:00:00Z",
};

const speaking = run(connected, {
  type: "request_started",
  id: "r1",
  kind: "audio",
  streamId: "stream-1",
});

describe("voiceReducer: transcripts and intents", () => {
  it("shows partial then final transcripts for the utterance in progress", () => {
    let s = run(speaking, server("stt.partial", { streamId: "stream-1", text: "opt" }, "r1"));
    expect(s.busy).toBe("listening");
    expect(s.partialTranscript).toBe("opt");

    s = run(
      s,
      { type: "capture_ended" },
      server("stt.final", { streamId: "stream-1", text: "option B" }, "r1")
    );
    expect(s.partialTranscript).toBe("");
    expect(s.finalTranscript).toBe("option B");
    expect(s.busy).toBe("processing");
  });

  it("does not announce partial transcripts", () => {
    const before = speaking.notice;
    const s = run(
      speaking,
      server("stt.partial", { streamId: "stream-1", text: "o" }, "r1"),
      server("stt.partial", { streamId: "stream-1", text: "op" }, "r1"),
      server("stt.partial", { streamId: "stream-1", text: "opt" }, "r1")
    );
    expect(s.notice).toBe(before);
  });

  it("ignores transcripts, intents and responses that answer an older request", () => {
    const s = run(
      speaking,
      { type: "request_started", id: "r2", kind: "command" },
      server("stt.partial", { streamId: "stream-1", text: "late" }, "r1"),
      server("stt.final", { streamId: "stream-1", text: "late" }, "r1"),
      server("intent", { intent: "next_question", confidence: 1, classifier: "rules" }, "r1"),
      server(
        "response",
        { responseId: "x", kind: "question", text: "Old question", audio: false },
        "r1"
      )
    );
    expect(s.partialTranscript).toBe("");
    expect(s.finalTranscript).toBe("");
    expect(s.intent).toBeNull();
    expect(s.response).toBeNull();
    expect(s.busy).toBe("processing");
  });

  it("records the backend's interpretation without acting on it", () => {
    const s = run(
      speaking,
      server(
        "intent",
        { intent: "select_option", optionKey: "B", confidence: 0.9, classifier: "rules" },
        "r1"
      )
    );
    expect(s.intent).toEqual({ name: "select_option", optionKey: "B" });
    expect(s.pending).toBeNull();
    expect(s.answer.phase).toBe("idle");
  });

  it("announces a clarification as text only when there is no audio for it", () => {
    const text = "Sorry, I didn't understand.";
    const silent = run(
      speaking,
      server(
        "response",
        { responseId: "x", kind: "clarification", code: "not_understood", text, audio: false },
        "r1"
      )
    );
    expect(silent.notice?.text).toBe(text);
    expect(silent.busy).toBe("idle");

    const spoken = run(
      speaking,
      server("response", { responseId: "x", kind: "clarification", text, audio: true }, "r1")
    );
    expect(spoken.response?.text).toBe(text);
    expect(spoken.notice?.text).not.toBe(text);
  });

  it("falls back to announcing the text when the audio fails", () => {
    const s = run(
      speaking,
      server(
        "response",
        { responseId: "x", kind: "question", text: "Question 1", audio: true },
        "r1"
      ),
      server("tts.end", { responseId: "x", reason: "error", code: "tts_failed", chunks: 0 }, "r1")
    );
    expect(s.notice?.text).toBe("Question 1");
  });
});

describe("voiceReducer: answer confirmation and persistence", () => {
  const staged = run(speaking, server("confirm.request", confirmRequest, "r1"));

  it("treats a staged answer as awaiting confirmation, not saved", () => {
    expect(staged.pending).toEqual(confirmRequest);
    expect(staged.answer).toMatchObject({ phase: "awaiting_confirmation", optionKey: "B" });
  });

  it("is saving after a confirm is sent, and saved only on the backend's result", () => {
    let s = run(staged, { type: "request_started", id: "r2", kind: "command", intent: "confirm" });
    expect(s.answer.phase).toBe("saving");
    expect(s.pending).not.toBeNull();

    s = run(
      s,
      server(
        "action.result",
        {
          action: "answer",
          status: "ok",
          saved: true,
          confirmationId: "conf-1",
          questionId: QUESTION_1.id,
          optionKey: "B",
        },
        "r2"
      )
    );
    expect(s.answer).toMatchObject({ phase: "saved", questionId: QUESTION_1.id, optionKey: "B" });
    expect(s.pending).toBeNull();
    expect(s.notice?.text).toBe("Answer saved.");
  });

  it("is saving when the backend reports a spoken yes", () => {
    const s = run(
      staged,
      { type: "request_started", id: "r2", kind: "audio", streamId: "stream-2" },
      server("intent", { intent: "confirm", confidence: 1, classifier: "rules" }, "r2")
    );
    expect(s.answer.phase).toBe("saving");
  });

  it("never reports saved for a result that does not say saved", () => {
    const s = run(
      staged,
      server("action.result", { action: "answer", status: "ok", saved: false }, "r1")
    );
    expect(s.answer.phase).toBe("awaiting_confirmation");
  });

  it("keeps the confirmation open after a retryable failure and closes it otherwise", () => {
    const retryable = run(
      staged,
      server(
        "action.result",
        {
          action: "answer",
          status: "failed",
          saved: false,
          code: "service_unavailable",
          retryable: true,
        },
        "r2"
      )
    );
    expect(retryable.answer).toMatchObject({ phase: "failed", retryable: true });
    expect(retryable.pending).not.toBeNull();
    expect(retryable.notice?.tone).toBe("error");

    const final = run(
      staged,
      server(
        "action.result",
        {
          action: "answer",
          status: "failed",
          saved: false,
          code: "invalid_option",
          retryable: false,
        },
        "r2"
      )
    );
    expect(final.answer.phase).toBe("failed");
    expect(final.pending).toBeNull();
  });

  it("clears the staged answer when the backend cancels it", () => {
    const s = run(
      staged,
      server(
        "action.result",
        { action: "answer", status: "cancelled", saved: false, confirmationId: "conf-1" },
        "r2"
      )
    );
    expect(s.pending).toBeNull();
    expect(s.answer.phase).toBe("cancelled");
  });

  it("says an answer was replaced, and keeps the new one staged", () => {
    const replaced = run(
      staged,
      server(
        "action.result",
        {
          action: "answer",
          status: "cancelled",
          saved: false,
          code: "superseded",
          confirmationId: "conf-1",
        },
        "r2"
      )
    );
    expect(replaced.notice?.text).toBe(
      "Your earlier answer was replaced by the new one and was not saved."
    );

    const next = run(
      replaced,
      server("confirm.request", { ...confirmRequest, confirmationId: "conf-2" }, "r2")
    );
    expect(next.pending?.confirmationId).toBe("conf-2");
    expect(next.answer.phase).toBe("awaiting_confirmation");
  });

  it("handles a duplicate saved result idempotently", () => {
    const result = server(
      "action.result",
      {
        action: "answer",
        status: "ok",
        saved: true,
        duplicate: true,
        confirmationId: "conf-1",
        questionId: QUESTION_1.id,
      },
      "r3"
    );
    const once = run(staged, result);
    const twice = run(once, result);
    expect(twice.answer).toEqual(once.answer);
    expect(twice.pending).toBeNull();
  });

  it("applies results for older requests, because they describe durable state", () => {
    const s = run(
      staged,
      { type: "request_started", id: "r2", kind: "command", intent: "confirm" },
      { type: "request_started", id: "r3", kind: "command", intent: "repeat_question" },
      server(
        "action.result",
        {
          action: "answer",
          status: "ok",
          saved: true,
          confirmationId: "conf-1",
          questionId: QUESTION_1.id,
        },
        "r2"
      )
    );
    expect(s.answer.phase).toBe("saved");
  });

  it("drops a staged answer when the session moves to another question", () => {
    const same = run(
      staged,
      server("session.state", { status: "IN_PROGRESS", question: QUESTION_1 })
    );
    expect(same.pending).not.toBeNull();
    const moved = run(
      staged,
      server("session.state", { status: "IN_PROGRESS", question: QUESTION_2 })
    );
    expect(moved.pending).toBeNull();
    expect(moved.answer.phase).toBe("idle");
  });

  it("expires only the confirmation it was started for", () => {
    expect(
      run(staged, { type: "confirmation_expired", confirmationId: "other" }).pending
    ).not.toBeNull();
    const expired = run(staged, { type: "confirmation_expired", confirmationId: "conf-1" });
    expect(expired.pending).toBeNull();
    expect(expired.answer.phase).toBe("cancelled");
    expect(expired.notice?.text).toMatch(/not saved/i);
  });

  it("marks an in-flight save as unconfirmed if the connection drops", () => {
    const s = run(
      staged,
      { type: "request_started", id: "r2", kind: "command", intent: "confirm" },
      { type: "connection", status: "reconnecting", attempt: 1 }
    );
    expect(s.answer).toMatchObject({ phase: "failed", retryable: true });
    expect(s.pending).toBeNull();
    expect(s.busy).toBe("idle");
  });

  it("does not carry a staged answer across a reconnect", () => {
    const s = run(staged, server("ready", readyPayload({ resumed: true })));
    expect(s.pending).toBeNull();
    expect(s.answer.phase).toBe("idle");
    expect(s.notice?.text).toBe("Voice assistant reconnected.");
  });
});

describe("voiceReducer: errors and connection", () => {
  it("ends the turn and explains recognition failures for the active stream", () => {
    const s = run(
      speaking,
      server(
        "error",
        { code: "stt_timeout", message: "x", retryable: true, streamId: "stream-1" },
        "r1"
      )
    );
    expect(s.busy).toBe("idle");
    expect(s.activeRequestId).toBeNull();
    expect(s.notice).toMatchObject({ tone: "error" });
    expect(s.notice?.text).toMatch(/try again/i);
  });

  it("ignores errors about requests that are no longer current", () => {
    const s = run(
      speaking,
      { type: "request_started", id: "r2", kind: "command" },
      server(
        "error",
        { code: "turn_cancelled", message: "x", retryable: true, streamId: "stream-1" },
        "r1"
      ),
      server(
        "error",
        { code: "stt_failed", message: "x", retryable: true, streamId: "stream-1" },
        "r1"
      ),
      server("error", { code: "duplicate_request", message: "x", retryable: false }, "old")
    );
    expect(s.busy).toBe("processing");
    expect(s.activeRequestId).toBe("r2");
    expect(s.notice?.tone).not.toBe("error");
  });

  it("always surfaces session-level errors", () => {
    const s = run(
      speaking,
      server("error", { code: "session_expired", message: "x", retryable: false }, "something-else")
    );
    expect(s.notice?.text).toBe("The exam time has expired.");
  });

  it("never shows the server's raw error message", () => {
    const s = run(
      speaking,
      server(
        "error",
        { code: "internal_error", message: "pq: connection refused at 10.0.0.5", retryable: true },
        "r1"
      )
    );
    expect(s.notice?.text).not.toContain("pq:");
  });

  it("describes each way the connection can end", () => {
    const lost = run(connected, { type: "connection", status: "reconnecting", attempt: 1 });
    expect(lost.notice?.text).toMatch(/reconnecting/i);
    expect(lost.attempt).toBe(1);

    const gaveUp = run(lost, { type: "connection", status: "disconnected" });
    expect(gaveUp.notice?.text).toMatch(/reconnect button/i);

    const ended = run(connected, {
      type: "connection",
      status: "error",
      fatal: "session_ended",
      sessionReason: "submitted",
    });
    expect(ended.fatal).toBe("session_ended");
    expect(ended.sessionReason).toBe("submitted");
    expect(ended.notice?.text).toMatch(/session has ended/i);

    const replaced = run(connected, { type: "connection", status: "error", fatal: "replaced" });
    expect(replaced.notice?.text).toMatch(/another tab/i);
  });

  it("gives every notice a new sequence number so repeats are re-announced", () => {
    const a = run(connected, { type: "notice", tone: "info", text: "Same" });
    const b = run(a, { type: "notice", tone: "info", text: "Same" });
    expect(b.notice!.seq).toBeGreaterThan(a.notice!.seq);
  });

  it("ignores server messages and status changes once voice is turned off", () => {
    const off = run(connected, { type: "disabled" });
    expect(off.enabled).toBe(false);
    const s = run(off, server("confirm.request", confirmRequest), {
      type: "connection",
      status: "connected",
    });
    expect(s.pending).toBeNull();
    expect(s.connection).toBe("idle");
  });

  it("tells the student once when audio is blocked", () => {
    const blocked = run(connected, { type: "speech", state: "blocked" });
    expect(blocked.notice?.text).toMatch(/blocked/i);
    const again = run(blocked, { type: "speech", state: "blocked" });
    expect(again.notice).toBe(blocked.notice);
  });
});
