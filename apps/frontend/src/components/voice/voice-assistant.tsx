"use client";

import { useEffect } from "react";
import { VoicePanel } from "@/components/voice/voice-panel";
import { useVoiceSession, type UseVoiceSessionOptions } from "@/hooks/use-voice-session";

export type VoiceAssistantProps = UseVoiceSessionOptions;

/**
 * The voice assistant for one exam session: connection, microphone, speech
 * and the panel that controls them. Mount it only while the session is in
 * progress; unmounting releases the socket, the microphone and all audio.
 */
export function VoiceAssistant(props: VoiceAssistantProps) {
  const voice = useVoiceSession(props);
  const { state, startSpeaking, finishSpeaking, stopSpeech } = voice;
  const enabled = state.enabled;
  const recording = state.mic === "recording";

  useEffect(() => {
    if (!enabled) return;
    function handleKeyDown(e: KeyboardEvent) {
      if (!e.altKey || e.ctrlKey || e.metaKey) return;
      const key = e.key.toLowerCase();
      if (key === "m") {
        e.preventDefault();
        if (recording) finishSpeaking();
        else void startSpeaking();
      } else if (key === "s") {
        e.preventDefault();
        stopSpeech();
      }
    }
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [enabled, recording, startSpeaking, finishSpeaking, stopSpeech]);

  return <VoicePanel voice={voice} disabled={props.disabled} />;
}

export default VoiceAssistant;
