"use client";

import { useEffect, useRef, useState, type FocusEvent } from "react";
import {
  AlertTriangle,
  Check,
  Loader2,
  Mic,
  Power,
  RotateCcw,
  Square,
  Volume2,
  VolumeX,
  X,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import type { VoiceController } from "@/hooks/use-voice-session";
import type { ConnectionStatus } from "@/services/voice/voice-socket";
import type { VoiceState } from "@/services/voice/voice-state";

export interface VoicePanelProps {
  voice: VoiceController;
  /** True while the page is navigating or submitting. */
  disabled?: boolean;
  /** True when the current question takes a written answer, which can be dictated. */
  allowVoiceAnswer?: boolean;
  /** Whether each new question is read aloud when the page moves to it. */
  autoRead?: boolean;
  onAutoReadChange?(on: boolean): void;
}

const CONNECTION_LABEL: Record<ConnectionStatus, string> = {
  idle: "Off",
  connecting: "Connecting",
  connected: "Connected",
  reconnecting: "Reconnecting",
  disconnected: "Disconnected",
  error: "Stopped",
};

function connectionVariant(
  status: ConnectionStatus
): "default" | "secondary" | "destructive" | "outline" {
  if (status === "connected") return "default";
  if (status === "disconnected" || status === "error") return "destructive";
  return "secondary";
}

function activityText(state: VoiceState): string {
  if (state.mic === "requesting") return "Waiting for microphone permission…";
  if (state.mic === "recording") {
    return state.captureMode === "answer"
      ? "Listening to your answer. Press Stop and send when you have finished."
      : "Listening for a command. Press Stop and send when you have finished.";
  }
  if (state.busy === "processing") return "Processing…";
  if (state.speech === "playing") return "Speaking…";
  return "Ready";
}

function answerText(
  state: VoiceState,
  pendingShown: boolean
): { text: string; tone: "muted" | "ok" | "warn" | "error" } | null {
  switch (state.answer.phase) {
    case "awaiting_confirmation":
      // A staged answer for a question the page has moved away from is not
      // offered, so neither is its status.
      if (!pendingShown) return null;
      return { text: "Waiting for your confirmation. Not saved yet.", tone: "warn" };
    case "saving":
      return { text: "Saving your answer…", tone: "muted" };
    case "saved":
      return { text: "Answer saved. Confirmed by the exam server.", tone: "ok" };
    case "failed":
      return {
        text: state.answer.retryable
          ? "Your answer could not be confirmed as saved."
          : "Your answer was not saved.",
        tone: "error",
      };
    case "cancelled":
      return { text: "Answer not saved.", tone: "muted" };
    default:
      return null;
  }
}

const TONE_CLASS = {
  muted: "text-muted-foreground",
  ok: "text-emerald-700 dark:text-emerald-400",
  warn: "text-amber-700 dark:text-amber-400",
  error: "text-destructive",
} as const;

/** The server rejects answers longer than this. */
const MAX_ANSWER_CHARS = 2000;

/**
 * The answer waiting for a yes or no. A dictated answer can be corrected here
 * before it is saved: the student's version is what is kept, and the original
 * recording and what the recognizer heard are kept alongside it. A chosen
 * option has nothing to correct.
 */
function PendingAnswer({
  pending,
  canAct,
  saving,
  recordingKept,
  onConfirm,
  onDeny,
}: {
  pending: NonNullable<VoiceController["pending"]>;
  canAct: boolean;
  saving: boolean;
  recordingKept: boolean;
  onConfirm(text?: string): void;
  onDeny(): void;
}) {
  const dictated = !pending.optionKey;
  const [draft, setDraft] = useState(pending.text ?? "");
  const changed = dictated && draft.trim() !== (pending.text ?? "").trim();
  const empty = dictated && draft.trim() === "";

  return (
    <div
      role="group"
      aria-labelledby="voice-confirm-title"
      className="rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 flex flex-col gap-2"
      data-testid="voice-confirmation"
    >
      <p id="voice-confirm-title" className="font-semibold">
        Confirm your answer
      </p>
      {dictated ? (
        <>
          <label htmlFor="voice-answer-draft" className="text-sm font-medium">
            This is what was heard. Correct it here if it is wrong.
          </label>
          <textarea
            id="voice-answer-draft"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            maxLength={MAX_ANSWER_CHARS}
            rows={4}
            aria-describedby="voice-answer-draft-help"
            aria-invalid={empty}
            disabled={saving}
            className="w-full rounded-md border bg-background p-2 text-base leading-relaxed"
            data-testid="voice-answer-draft"
          />
          <p id="voice-answer-draft-help" className="text-xs text-muted-foreground">
            {changed
              ? "You have changed the text. Your version will be saved."
              : "If the text is right, just save it."}
            {recordingKept && " Your original recording is kept either way."}
          </p>
          {empty && (
            <p role="alert" className="text-xs text-destructive" data-testid="voice-answer-empty">
              An answer cannot be empty.
            </p>
          )}
        </>
      ) : (
        <p>
          Option {pending.optionKey}
          {pending.optionText ? `: ${pending.optionText}` : ""}
        </p>
      )}
      <p className="text-xs text-muted-foreground">
        This answer is not saved yet. Say yes or no, or use the buttons.
      </p>
      <div className="flex flex-wrap gap-2">
        <Button
          size="sm"
          onClick={() => onConfirm(changed ? draft : undefined)}
          disabled={!canAct || saving || empty}
          className="gap-1.5"
          data-testid="voice-confirm-yes"
        >
          <Check className="size-3.5" aria-hidden="true" />
          {changed ? "Save my corrected answer" : "Yes, save answer"}
        </Button>
        <Button
          size="sm"
          variant="outline"
          onClick={onDeny}
          disabled={!canAct || saving}
          className="gap-1.5"
          data-testid="voice-confirm-no"
        >
          <X className="size-3.5" aria-hidden="true" />
          No, cancel
        </Button>
      </div>
    </div>
  );
}

/**
 * Controls and feedback for the voice assistant. Everything here is also
 * operable by keyboard, and every voice action has an on-screen equivalent.
 */
export function VoicePanel({
  voice,
  disabled = false,
  allowVoiceAnswer = false,
  autoRead = false,
  onAutoReadChange,
}: VoicePanelProps) {
  const { state, pending } = voice;
  const connected = state.connection === "connected";
  const recording = state.mic === "recording";
  const requesting = state.mic === "requesting";
  const sttAvailable = state.capabilities?.stt === true;
  const canAct = connected && !disabled;
  const answer = answerText(state, pending !== null);
  const canRetryConnection =
    state.connection === "disconnected" ||
    (state.connection === "error" && (state.fatal === "replaced" || state.fatal === "protocol"));

  // Starting or stopping a recording swaps the Start and Stop buttons. When
  // focus was on them, it moves to the button that replaced them instead of
  // falling back to the top of the page.
  const controlsRef = useRef<HTMLDivElement>(null);
  const focusInControlsRef = useRef(false);
  const trackFocusLeaving = (e: FocusEvent<HTMLDivElement>) => {
    const next = e.relatedTarget as Node | null;
    if (next ? !e.currentTarget.contains(next) : e.target.isConnected) {
      focusInControlsRef.current = false; // the student moved focus elsewhere
    }
  };
  useEffect(() => {
    if (!focusInControlsRef.current) return;
    const active = document.activeElement;
    if (active && active !== document.body && controlsRef.current?.contains(active)) return;
    controlsRef.current?.querySelector<HTMLElement>('[data-testid="voice-mic-button"]')?.focus();
  }, [recording]);

  return (
    <Card className="bg-background shadow-xs border" data-testid="voice-panel">
      <CardHeader className="py-3 px-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <CardTitle className="text-sm font-semibold" id="voice-panel-title">
              Voice assistant
            </CardTitle>
            <Badge
              variant={connectionVariant(state.connection)}
              data-testid="voice-connection-status"
            >
              {state.connection === "reconnecting" && state.attempt > 0
                ? `Reconnecting (attempt ${state.attempt})`
                : CONNECTION_LABEL[state.connection]}
            </Badge>
            {recording && (
              <Badge
                variant="destructive"
                className="gap-1.5"
                data-testid="voice-recording-indicator"
              >
                <span
                  className="size-2 rounded-full bg-current motion-safe:animate-pulse"
                  aria-hidden="true"
                />
                Recording
              </Badge>
            )}
          </div>
          {state.enabled ? (
            <Button variant="outline" size="sm" onClick={voice.disable} className="gap-1.5">
              <Power className="size-3.5" aria-hidden="true" />
              Turn off voice
            </Button>
          ) : (
            <Button
              size="sm"
              onClick={voice.enable}
              className="gap-1.5"
              data-testid="voice-enable-button"
            >
              <Power className="size-3.5" aria-hidden="true" />
              Turn on voice assistant
            </Button>
          )}
        </div>
      </CardHeader>

      <CardContent
        className="px-4 pb-4 flex flex-col gap-3 text-sm"
        aria-labelledby="voice-panel-title"
      >
        {!state.enabled && (
          <p className="text-muted-foreground">
            Turn on the voice assistant to have questions read aloud and to answer by speaking. Your
            microphone is used only while you are recording, and your browser will ask for
            permission the first time. You can always use the on-screen buttons and keyboard
            instead.
          </p>
        )}

        {state.enabled && (
          <>
            {canRetryConnection && (
              <div className="flex flex-wrap items-center gap-3 rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-destructive">
                <AlertTriangle className="size-4 shrink-0" aria-hidden="true" />
                <span className="flex-1">
                  {state.fatal === "replaced"
                    ? "Voice is active in another tab or device."
                    : "The voice connection was lost."}
                </span>
                <Button
                  size="sm"
                  variant="destructive"
                  onClick={voice.reconnect}
                  className="gap-1.5"
                >
                  <RotateCcw className="size-3.5" aria-hidden="true" />
                  Reconnect
                </Button>
              </div>
            )}

            <div
              ref={controlsRef}
              className="flex flex-wrap items-center gap-2"
              role="group"
              aria-label="Voice controls"
              onFocusCapture={() => {
                focusInControlsRef.current = true;
              }}
              onBlurCapture={trackFocusLeaving}
            >
              {recording ? (
                <>
                  <Button
                    size="sm"
                    onClick={voice.finishSpeaking}
                    aria-pressed="true"
                    className="gap-1.5"
                    data-testid="voice-mic-button"
                  >
                    <Square className="size-3.5 fill-current" aria-hidden="true" />
                    Stop and send
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={voice.cancelSpeaking}
                    className="gap-1.5"
                  >
                    <X className="size-3.5" aria-hidden="true" />
                    Discard recording
                  </Button>
                </>
              ) : (
                <Button
                  size="sm"
                  onClick={() => void voice.startSpeaking()}
                  disabled={!canAct || !sttAvailable || requesting}
                  aria-pressed="false"
                  aria-keyshortcuts="Alt+M"
                  className="gap-1.5"
                  data-testid="voice-mic-button"
                >
                  {requesting ? (
                    <Loader2 className="size-3.5 motion-safe:animate-spin" aria-hidden="true" />
                  ) : (
                    <Mic className="size-3.5" aria-hidden="true" />
                  )}
                  Start speaking
                </Button>
              )}

              {!recording && allowVoiceAnswer && (
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => void voice.startSpeaking("answer")}
                  disabled={!canAct || !sttAvailable || requesting}
                  aria-keyshortcuts="Alt+D"
                  className="gap-1.5"
                  data-testid="voice-dictate-button"
                >
                  <Mic className="size-3.5" aria-hidden="true" />
                  Dictate answer
                </Button>
              )}

              <Button
                size="sm"
                variant="outline"
                onClick={() => voice.sendCommand("repeat_question")}
                disabled={!canAct}
              >
                Read question
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={() => voice.sendCommand("read_back_answer")}
                disabled={!canAct}
              >
                Read my answer
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={() => voice.sendCommand("time_remaining")}
                disabled={!canAct}
              >
                Time left
              </Button>

              {state.speech === "playing" && (
                <Button size="sm" variant="outline" onClick={voice.stopSpeech} className="gap-1.5">
                  <VolumeX className="size-3.5" aria-hidden="true" />
                  Stop speaking
                </Button>
              )}
              {(voice.canReplay || state.speech === "blocked") && state.speech !== "playing" && (
                <Button
                  size="sm"
                  variant="outline"
                  onClick={voice.replaySpeech}
                  className="gap-1.5"
                >
                  <Volume2 className="size-3.5" aria-hidden="true" />
                  {state.speech === "blocked" ? "Play response" : "Replay response"}
                </Button>
              )}
            </div>

            {onAutoReadChange && state.capabilities?.tts && (
              <label className="flex items-center gap-2 text-sm">
                <Switch
                  checked={autoRead}
                  onCheckedChange={onAutoReadChange}
                  aria-label="Read each new question aloud"
                  data-testid="voice-auto-read"
                />
                <span>Read each new question aloud</span>
              </label>
            )}

            {state.capabilities?.recording === true && (
              <p className="text-muted-foreground" data-testid="voice-recording-notice">
                Audio of answers you dictate is recorded and kept so your teacher can check the
                transcript against what you said. Voice commands are not recorded.
                {state.recording === "stored" && (
                  <span className="block font-medium" data-testid="voice-recording-status">
                    The audio of your last dictated answer was saved.
                  </span>
                )}
                {state.recording === "failed" && (
                  <span className="block font-medium text-destructive" data-testid="voice-recording-status">
                    The audio of your last dictated answer could not be saved. Your answer text is
                    not affected.
                  </span>
                )}
              </p>
            )}

            {connected && state.capabilities && !sttAvailable && (
              <p className="text-muted-foreground">
                Speech recognition is not available on this server. The buttons above and the
                on-screen exam controls still work.
              </p>
            )}

            {state.mic === "denied" && (
              <p className="text-destructive" data-testid="voice-mic-help">
                Microphone access is blocked. Open your browser&apos;s site settings for this page,
                allow the microphone, then press Start speaking again. You can keep using the
                on-screen controls in the meantime.
              </p>
            )}
            {state.mic === "no_device" && (
              <p className="text-destructive">
                No microphone was found. Connect one, then press Start speaking again.
              </p>
            )}
            {state.mic === "unsupported" && (
              <p className="text-destructive">
                This browser cannot record audio. Use the on-screen controls, or try a current
                version of Chrome, Edge, Firefox or Safari.
              </p>
            )}
            {state.mic === "error" && (
              <p className="text-destructive">
                The microphone stopped working. Check it, then press Start speaking again.
              </p>
            )}

            <p className="text-muted-foreground" data-testid="voice-activity">
              {connected ? activityText(state) : CONNECTION_LABEL[state.connection]}
            </p>

            {/* Interim text changes many times a second, so it is shown but never announced. */}
            {(state.partialTranscript || state.finalTranscript) && (
              <p aria-live="off" data-testid="voice-transcript">
                <span className="font-semibold">You said: </span>
                {state.finalTranscript ? (
                  <span>{state.finalTranscript}</span>
                ) : (
                  <span className="italic text-muted-foreground">{state.partialTranscript}</span>
                )}
              </p>
            )}

            {state.response && (
              <p data-testid="voice-response">
                <span className="font-semibold">Assistant: </span>
                {state.response.text}
              </p>
            )}

            {pending && (
              <PendingAnswer
                key={pending.confirmationId}
                pending={pending}
                canAct={canAct}
                saving={state.answer.phase === "saving"}
                recordingKept={state.capabilities?.recording === true}
                onConfirm={voice.confirm}
                onDeny={voice.deny}
              />
            )}

            {answer && (
              <p
                className={`font-medium ${TONE_CLASS[answer.tone]}`}
                data-testid="voice-answer-status"
              >
                {answer.text}
              </p>
            )}

            {state.notice?.tone === "error" && (
              <p className="text-destructive" data-testid="voice-error">
                {state.notice.text}
              </p>
            )}

            <p className="text-xs text-muted-foreground">
              Keyboard: Enter, when no button or field has focus, {allowVoiceAnswer ? "dictates your answer" : "starts a voice command"}, and Enter again stops. Alt+M starts or stops a voice command. {allowVoiceAnswer ? "Alt+D dictates your answer. " : ""}Escape discards a recording. Alt+S stops speech. Left and Right arrows change question when no button or field has focus; Alt+N and Alt+P do too, except while you are typing in a field.
              Words spoken while dictating are your answer, never commands.
            </p>
          </>
        )}

        {/*
          Screen-reader announcements. One short message per event: status
          changes, results and errors. Audio chunks and interim transcripts are
          never announced. The key makes a repeated message announce again.
        */}
        <div className="sr-only" role="status" aria-live="polite" data-testid="voice-live-status">
          {state.notice?.tone === "info" && <span key={state.notice.seq}>{state.notice.text}</span>}
        </div>
        <div className="sr-only" role="alert" data-testid="voice-live-alert">
          {state.notice?.tone === "error" && (
            <span key={state.notice.seq}>{state.notice.text}</span>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

export default VoicePanel;
