"use client";

import { useEffect, useRef, useState } from "react";
import { VoicePanel } from "@/components/voice/voice-panel";
import { useVoiceSession, type UseVoiceSessionOptions } from "@/hooks/use-voice-session";
import {
  hasNoModifiers,
  isInsideLayer,
  isPlainShortcutEvent,
  isSurfaceTarget,
  shortcutLetter,
} from "@/lib/keyboard";

/** What the voice assistant is doing that must not be interrupted by leaving the question. */
export interface VoiceActivity {
  /** The microphone is open, or permission is being asked for. */
  recording: boolean;
  /** A recording was sent and the result has not come back yet. */
  processing: boolean;
  /** An answer is staged and waiting for a yes or no. Not saved. */
  awaitingConfirmation: boolean;
}

export type VoiceAssistantProps = UseVoiceSessionOptions & {
  /** The current question takes a written answer, so voice dictation is offered. */
  allowVoiceAnswer?: boolean;
  /** Called whenever {@link VoiceActivity} changes. */
  onActivityChange?(activity: VoiceActivity): void;
};

const AUTO_READ_KEY = "scribe.autoReadQuestions";

/** The student's choice is a per-browser convenience; storage may be unavailable. */
function loadAutoRead(): boolean {
  try {
    return window.localStorage.getItem(AUTO_READ_KEY) !== "off";
  } catch {
    return true;
  }
}

/**
 * Enter presses closer together than this count as one. It absorbs a double
 * tap and a delayed or duplicated key event, and means a recording cannot be
 * stopped before it has really started.
 */
export const TOGGLE_LOCK_MS = 300;

/**
 * The voice assistant for one exam session: connection, microphone, speech
 * and the panel that controls them. Mount it only while the session is in
 * progress; unmounting releases the socket, the microphone and all audio.
 *
 * Keyboard:
 *  - Enter starts and stops recording, but only when focus is on the page
 *    itself and not on a control, so it never fires a button or types a line
 *    break. On a written-answer question it dictates the answer; otherwise it
 *    listens for a command.
 *  - Escape discards a recording in progress, unless a dialog has the keyboard.
 *  - Alt+M starts or stops a voice command, Alt+D dictates an answer and Alt+S
 *    stops speech. These work wherever focus is, except inside a dialog. They
 *    match the physical key, so Option+M works on a Mac.
 */
export function VoiceAssistant(props: VoiceAssistantProps) {
  const { allowVoiceAnswer = false, onActivityChange, ...sessionOptions } = props;
  const [autoRead, setAutoRead] = useState(true);
  useEffect(() => setAutoRead(loadAutoRead()), []);
  const changeAutoRead = (on: boolean) => {
    setAutoRead(on);
    try {
      window.localStorage.setItem(AUTO_READ_KEY, on ? "on" : "off");
    } catch {
      // The choice still applies for this visit.
    }
  };
  const voice = useVoiceSession({ ...sessionOptions, autoReadQuestions: autoRead });
  const { state, startSpeaking, finishSpeaking, cancelSpeaking, stopSpeech, announce } = voice;
  const enabled = state.enabled;
  const recording = state.mic === "recording";
  const requesting = state.mic === "requesting";
  const processing = state.busy === "processing";
  const awaitingConfirmation = voice.pending !== null;

  // Report activity to the page, which uses it to stop navigation from
  // silently discarding a recording or an unconfirmed answer.
  const activityRef = useRef(onActivityChange);
  useEffect(() => {
    activityRef.current = onActivityChange;
  });
  useEffect(() => {
    activityRef.current?.({
      recording: recording || requesting,
      processing,
      awaitingConfirmation,
    });
  }, [recording, requesting, processing, awaitingConfirmation]);

  const lastToggleRef = useRef(0);

  useEffect(() => {
    if (!enabled) return;

    function handleKeyDown(e: KeyboardEvent) {
      if (!isPlainShortcutEvent(e)) return;

      if (e.key === "Enter" && hasNoModifiers(e)) {
        // Enter belongs to the control that has focus. Only act for the page.
        if (!isSurfaceTarget(e.target)) return;
        e.preventDefault();

        const now = Date.now();
        if (now - lastToggleRef.current < TOGGLE_LOCK_MS) return;
        lastToggleRef.current = now;

        if (recording) {
          finishSpeaking();
          return;
        }
        if (requesting) return;
        // startSpeaking refuses, and says why, while an answer waits for
        // confirmation or the last request is still being processed.
        void startSpeaking(allowVoiceAnswer ? "answer" : "command");
        return;
      }

      if (e.key === "Escape" && hasNoModifiers(e)) {
        // Escape closes a dialog first; it only discards a recording when no
        // dialog, menu or list has the keyboard.
        if (!(recording || requesting) || isInsideLayer(e.target)) return;
        e.preventDefault();
        cancelSpeaking();
        announce("Recording discarded. Nothing was sent.", "info");
        return;
      }

      // Alt shortcuts work wherever focus is, except inside a dialog (such as
      // the submit confirmation), which owns the keyboard.
      if (!e.altKey || e.ctrlKey || e.metaKey || isInsideLayer(e.target)) return;
      const key = shortcutLetter(e);
      if (key === "m" || (key === "d" && allowVoiceAnswer)) {
        e.preventDefault();
        lastToggleRef.current = Date.now(); // an Enter right after is the same press
        if (recording) finishSpeaking();
        else if (!requesting) void startSpeaking(key === "d" ? "answer" : "command");
      } else if (key === "s") {
        e.preventDefault();
        stopSpeech();
      }
    }
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [
    enabled,
    recording,
    requesting,
    allowVoiceAnswer,
    startSpeaking,
    finishSpeaking,
    cancelSpeaking,
    stopSpeech,
    announce,
  ]);

  return (
    <VoicePanel
      voice={voice}
      disabled={props.disabled}
      allowVoiceAnswer={allowVoiceAnswer}
      autoRead={autoRead}
      onAutoReadChange={changeAutoRead}
    />
  );
}

export default VoiceAssistant;
