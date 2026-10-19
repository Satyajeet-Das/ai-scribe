"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Loader2, Pause, Play, RotateCcw, SkipBack, SkipForward } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ApiError } from "@/services/api";

const SKIP_SECONDS = 5;
const SPEEDS = [0.75, 1, 1.25, 1.5] as const;

type Phase = "idle" | "loading" | "ready" | "error";

interface PlaybackProblem {
  message: string;
  /** Trying again could help. */
  retryable: boolean;
}

/** Student-facing text for each way fetching a recording can fail. */
export function describePlaybackError(err: unknown): PlaybackProblem {
  if (err instanceof ApiError) {
    if (err.status === 410) {
      return {
        message: "This recording has been removed. The transcript is still available.",
        retryable: false,
      };
    }
    if (err.status === 404) {
      return { message: "This recording could not be found.", retryable: false };
    }
    if (err.status === 403) {
      return { message: "You do not have permission to listen to this recording.", retryable: false };
    }
    if (err.status === 401) {
      return { message: "Your session has expired. Please sign in again.", retryable: false };
    }
    if (err.status === 429) {
      return {
        message: "Too many recordings were played in a short time. Wait a minute, then try again.",
        retryable: true,
      };
    }
    if (err.status === 0) {
      return {
        message: "Could not reach the server. Check your connection and try again.",
        retryable: true,
      };
    }
  }
  return { message: "The recording cannot be played right now. Please try again.", retryable: true };
}

function formatTime(seconds: number): string {
  const s = Math.max(0, Math.floor(Number.isFinite(seconds) ? seconds : 0));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}

export interface RecordingPlayerProps {
  /** Names the recording for assistive technology, e.g. "recording 1 of question 3". */
  label: string;
  /** Length from the server, shown until the browser reports the real one. */
  durationMs: number;
  /** Fetches the audio. Called only when the reviewer asks to listen. */
  load(signal: AbortSignal): Promise<Blob>;
}

/**
 * Plays one kept recording. Nothing is fetched until Play is pressed, because
 * every fetch is an audited access to a student's voice. The audio is held in
 * memory only, never given a link, and released when the player goes away.
 */
export function RecordingPlayer({ label, durationMs, load }: RecordingPlayerProps) {
  const audioRef = useRef<HTMLAudioElement>(null);
  const urlRef = useRef<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  const [phase, setPhase] = useState<Phase>("idle");
  const [problem, setProblem] = useState<PlaybackProblem | null>(null);
  const [playing, setPlaying] = useState(false);
  const [current, setCurrent] = useState(0);
  const [duration, setDuration] = useState(durationMs / 1000);
  const [rate, setRate] = useState<number>(1);
  const [status, setStatus] = useState("");

  // The control that had focus is replaced as the player moves from Play to
  // loading to the playback controls (or an error). Focus follows it, so a
  // keyboard user is not sent back to the top of the page.
  const containerRef = useRef<HTMLDivElement>(null);
  const loadingRef = useRef<HTMLParagraphElement>(null);
  const toggleRef = useRef<HTMLButtonElement>(null);
  const problemRef = useRef<HTMLDivElement>(null);
  const keepFocusRef = useRef(false);
  useEffect(() => {
    if (!keepFocusRef.current) return;
    // Only while focus is still on the player (or was lost with the control
    // it was on): if the reviewer moved elsewhere meanwhile, it stays there.
    const active = document.activeElement;
    const onPlayer =
      !active || active === document.body || containerRef.current?.contains(active) === true;
    if (!onPlayer) {
      keepFocusRef.current = false;
      return;
    }
    if (phase === "loading") loadingRef.current?.focus();
    if (phase === "ready") toggleRef.current?.focus();
    if (phase === "error") {
      (problemRef.current?.querySelector("button") ?? problemRef.current)?.focus();
    }
    if (phase !== "loading") keepFocusRef.current = false;
  }, [phase]);

  const release = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    audioRef.current?.pause();
    if (urlRef.current) {
      URL.revokeObjectURL(urlRef.current);
      urlRef.current = null;
    }
  }, []);

  useEffect(() => release, [release]);

  const fetchAndPlay = useCallback(async () => {
    const audio = audioRef.current;
    if (!audio) return;
    keepFocusRef.current = containerRef.current?.contains(document.activeElement) ?? false;
    release();
    const controller = new AbortController();
    abortRef.current = controller;
    setPhase("loading");
    setProblem(null);
    setStatus("Loading recording.");
    try {
      const blob = await load(controller.signal);
      if (controller.signal.aborted) return;
      const url = URL.createObjectURL(blob);
      urlRef.current = url;
      audio.src = url;
      audio.playbackRate = rate;
      setPhase("ready");
      setStatus("Recording ready.");
      // A browser may refuse to start sound on its own; the Play button is
      // then all that is needed.
      await Promise.resolve(audio.play()).catch(() => undefined);
    } catch (err) {
      if (controller.signal.aborted) return;
      setProblem(describePlaybackError(err));
      setPhase("error");
      setStatus("");
    }
  }, [load, rate, release]);

  const toggle = () => {
    const audio = audioRef.current;
    if (!audio) return;
    if (audio.paused || audio.ended) {
      if (audio.ended) audio.currentTime = 0;
      void Promise.resolve(audio.play()).catch(() => undefined);
    } else {
      audio.pause();
    }
  };

  const seek = (seconds: number) => {
    const audio = audioRef.current;
    if (!audio) return;
    const target = Math.min(Math.max(0, seconds), duration || seconds);
    audio.currentTime = target;
    setCurrent(target);
  };

  const changeRate = (value: number) => {
    setRate(value);
    if (audioRef.current) audioRef.current.playbackRate = value;
  };

  return (
    <div ref={containerRef} className="flex flex-col gap-2" data-testid="recording-player">
      {/* No native controls: the controls below are labelled and keyboard operable. */}
      <audio
        ref={audioRef}
        preload="none"
        onPlay={() => setPlaying(true)}
        onPause={() => setPlaying(false)}
        onEnded={() => setPlaying(false)}
        onTimeUpdate={(e) => setCurrent(e.currentTarget.currentTime)}
        onLoadedMetadata={(e) => {
          const d = e.currentTarget.duration;
          if (Number.isFinite(d) && d > 0) setDuration(d);
        }}
        onError={() => {
          if (!urlRef.current) return;
          setProblem({
            message: "This browser could not play the recording.",
            retryable: false,
          });
          setPhase("error");
        }}
      />

      {phase === "idle" && (
        <div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => void fetchAndPlay()}
            aria-label={`Play ${label}`}
            className="gap-1.5"
          >
            <Play className="size-4" aria-hidden="true" />
            Play recording
            <span className="text-muted-foreground tabular-nums">({formatTime(duration)})</span>
          </Button>
        </div>
      )}

      {phase === "loading" && (
        <p
          ref={loadingRef}
          tabIndex={-1}
          className="flex items-center gap-2 text-sm text-muted-foreground focus:outline-none"
          aria-busy="true"
        >
          <Loader2 className="size-4 motion-safe:animate-spin" aria-hidden="true" />
          Loading recording…
        </p>
      )}

      {phase === "ready" && (
        <div role="group" aria-label={label} className="flex flex-wrap items-center gap-2">
          <Button
            ref={toggleRef}
            variant="outline"
            size="icon"
            onClick={toggle}
            aria-label={`${playing ? "Pause" : "Play"} ${label}`}
            aria-pressed={playing}
          >
            {playing ? (
              <Pause className="size-4" aria-hidden="true" />
            ) : (
              <Play className="size-4" aria-hidden="true" />
            )}
          </Button>
          <Button
            variant="outline"
            size="icon"
            onClick={() => seek(current - SKIP_SECONDS)}
            aria-label={`Back ${SKIP_SECONDS} seconds`}
          >
            <SkipBack className="size-4" aria-hidden="true" />
          </Button>
          <Button
            variant="outline"
            size="icon"
            onClick={() => seek(current + SKIP_SECONDS)}
            aria-label={`Forward ${SKIP_SECONDS} seconds`}
          >
            <SkipForward className="size-4" aria-hidden="true" />
          </Button>
          <input
            type="range"
            min={0}
            max={duration || 0}
            step={0.1}
            value={Math.min(current, duration || 0)}
            onChange={(e) => seek(Number(e.target.value))}
            aria-label={`Seek ${label}`}
            aria-valuetext={`${formatTime(current)} of ${formatTime(duration)}`}
            className="min-w-32 flex-1 accent-primary"
          />
          <span className="text-sm tabular-nums text-muted-foreground" aria-hidden="true">
            {formatTime(current)} / {formatTime(duration)}
          </span>
          <label className="flex items-center gap-1.5 text-sm text-muted-foreground">
            Speed
            <select
              value={rate}
              onChange={(e) => changeRate(Number(e.target.value))}
              className="h-8 rounded-md border bg-background px-2 text-sm text-foreground"
            >
              {SPEEDS.map((s) => (
                <option key={s} value={s}>
                  {s}×
                </option>
              ))}
            </select>
          </label>
        </div>
      )}

      {phase === "error" && problem && (
        <div
          ref={problemRef}
          role="alert"
          tabIndex={-1}
          className="flex flex-wrap items-center gap-3 rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-sm focus:outline-none"
        >
          <span className="flex-1">{problem.message}</span>
          {problem.retryable && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => void fetchAndPlay()}
              className="gap-1.5"
            >
              <RotateCcw className="size-3.5" aria-hidden="true" />
              Try again
            </Button>
          )}
        </div>
      )}

      <p className="sr-only" role="status" aria-live="polite">
        {status}
      </p>
    </div>
  );
}
