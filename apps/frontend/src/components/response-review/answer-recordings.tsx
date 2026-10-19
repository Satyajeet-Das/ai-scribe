"use client";

import { Badge } from "@/components/ui/badge";
import { RecordingPlayer } from "@/components/response-review/recording-player";
import { formatDateTime } from "@/components/response-review/status-badge";
import { responseReviewApi } from "@/services/response-review-api";
import type {
  ReviewAnswerSource,
  ReviewQuestion,
  ReviewRecording,
} from "@/types/response-review";

/** How the saved answer came to be, for the reviewer. Mouse selections need no label. */
export const SOURCE_LABEL: Partial<Record<ReviewAnswerSource, string>> = {
  TYPED: "Typed by the student",
  STT_CONFIRMED: "Spoken, and confirmed by the student after hearing the transcript",
  STUDENT_CORRECTION: "Spoken, then corrected by the student",
};

/** Lower-case words only, so differences in punctuation and spacing do not count. */
function words(text: string): string {
  return text
    .toLowerCase()
    .replace(/[^\p{L}\p{N}\s]/gu, " ")
    .split(/\s+/)
    .filter(Boolean)
    .join(" ");
}

function seconds(ms: number): string {
  const s = Math.round(ms / 1000);
  return s < 60 ? `${s} second${s === 1 ? "" : "s"}` : `${Math.floor(s / 60)} min ${s % 60} s`;
}

/**
 * The audio kept for one answer, with what the speech recognizer heard beside
 * it, so a teacher can check a doubtful transcript against the real voice.
 * It never claims audio exists when it does not: every missing, removed or
 * unconfirmed case says what it is.
 */
export function AnswerRecordings({
  question,
  examId,
  studentId,
}: {
  question: ReviewQuestion;
  examId: string;
  studentId: string;
}) {
  const recordings = question.recordings ?? [];

  if (recordings.length === 0) {
    // Multiple-choice selections are not recorded, so there is nothing to say.
    if (question.type === "MCQ") return null;
    return (
      <section aria-label="Recorded audio" data-testid="no-recordings">
        <p className="rounded-lg border border-dashed p-3 text-sm text-muted-foreground">
          No audio was recorded for this answer.
        </p>
      </section>
    );
  }

  const answerText = question.answer.textAnswer ?? "";

  return (
    <section aria-labelledby={`recordings-${question.id}`} className="flex flex-col gap-3">
      <div>
        <h3 id={`recordings-${question.id}`} className="text-sm font-semibold">
          Recorded audio
        </h3>
        <p className="text-xs text-muted-foreground">
          Speech recognition can make mistakes. Play the recording to hear what the student actually
          said. Playing a recording is logged.
        </p>
      </div>
      <ol className="flex flex-col gap-3">
        {recordings.map((r, i) => (
          <RecordingItem
            key={r.id}
            recording={r}
            index={i}
            total={recordings.length}
            question={question}
            answerText={answerText}
            examId={examId}
            studentId={studentId}
          />
        ))}
      </ol>
    </section>
  );
}

function RecordingItem({
  recording,
  index,
  total,
  question,
  answerText,
  examId,
  studentId,
}: {
  recording: ReviewRecording;
  index: number;
  total: number;
  question: ReviewQuestion;
  answerText: string;
  examId: string;
  studentId: string;
}) {
  const label = `recording ${index + 1} of ${total} for question ${question.questionNumber}`;
  const recorded = formatDateTime(recording.recordedAt);
  // Older servers do not say; then a confirmed recording is taken to support
  // the saved answer, as before.
  const supportsAnswer = recording.confirmed && recording.supportsCurrentAnswer !== false;
  const differs =
    supportsAnswer &&
    answerText.trim() !== "" &&
    recording.transcript.trim() !== "" &&
    words(recording.transcript) !== words(answerText);

  return (
    <li className="flex flex-col gap-3 rounded-lg border p-4" data-testid="recording-item">
      <div className="flex flex-wrap items-center gap-2">
        <h4 className="text-sm font-semibold">
          Recording {index + 1}
          {total > 1 && <span className="font-normal text-muted-foreground"> of {total}</span>}
        </h4>
        <span className="text-xs text-muted-foreground">
          {seconds(recording.durationMs)}
          {recorded && <> · {recorded}</>}
        </span>
        {supportsAnswer ? (
          <Badge variant="secondary">Used for the saved answer</Badge>
        ) : recording.confirmed ? (
          <Badge variant="outline">Confirmed earlier, no longer the saved answer</Badge>
        ) : (
          <Badge variant="outline">Not used for the saved answer</Badge>
        )}
        {recording.status === "REMOVED" && <Badge variant="destructive">Audio removed</Badge>}
      </div>

      {recording.status === "AVAILABLE" ? (
        <RecordingPlayer
          label={label}
          durationMs={recording.durationMs}
          load={(signal) =>
            responseReviewApi.getRecordingAudio(examId, studentId, recording.id, signal)
          }
        />
      ) : (
        <p className="text-sm text-muted-foreground" role="status">
          The audio of this recording has been removed. The transcript below is still available.
        </p>
      )}

      <div>
        <h5 className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          What the speech recognizer heard (not edited)
        </h5>
        {recording.transcript.trim() ? (
          <p
            className="whitespace-pre-wrap break-words text-base leading-relaxed"
            data-testid="recording-transcript"
          >
            {recording.transcript}
          </p>
        ) : (
          <p className="text-sm text-muted-foreground">No transcript was produced.</p>
        )}
      </div>

      {differs && (
        <p
          className="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-950 dark:border-amber-800 dark:bg-amber-950/40 dark:text-amber-100"
          data-testid="transcript-differs"
        >
          The saved answer is different from this transcript. The student may have corrected it
          before or after confirming it. The saved answer is the one that counts.
        </p>
      )}
    </li>
  );
}
