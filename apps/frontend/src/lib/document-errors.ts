import { ApiError } from "@/services/api";

export type DocumentErrorKind =
  | "network"
  | "auth"
  | "forbidden"
  | "notFound"
  | "validation"
  | "conflict"
  | "unavailable"
  | "rateLimited"
  | "server";

export interface DocumentError {
  kind: DocumentErrorKind;
  /** The server's error code when there was one (for example STALE_VERSION). */
  code?: string;
  /** Plain language, safe to show. Never a stack trace or raw server text. */
  message: string;
  /** Trying the same thing again might work. */
  retryable: boolean;
  /** Field-level problems the server reported, keyed by field path. */
  fields: Record<string, string>;
}

/** What a teacher should do about each code the document endpoints return. */
const BY_CODE: Record<string, { message: string; kind: DocumentErrorKind; retryable: boolean }> = {
  PAYLOAD_TOO_LARGE: {
    message: "This file is larger than the server accepts. Choose a smaller file or split the paper.",
    kind: "validation",
    retryable: false,
  },
  UNSUPPORTED_MEDIA_TYPE: {
    message: "Only PDF files can be uploaded.",
    kind: "validation",
    retryable: false,
  },
  INVALID_PDF: {
    message: "This PDF could not be read. It may be damaged, incomplete or password protected.",
    kind: "validation",
    retryable: false,
  },
  DOCUMENT_ALREADY_UPLOADED: {
    message: "This file has already been uploaded to this exam.",
    kind: "conflict",
    retryable: false,
  },
  TOO_MANY_DOCUMENTS: {
    message: "This exam already has the most documents it can hold. Delete one first.",
    kind: "conflict",
    retryable: false,
  },
  EXAM_NOT_DRAFT: {
    message: "This exam is no longer a draft, so questions can't be added to it. Revert it to draft first.",
    kind: "conflict",
    retryable: false,
  },
  INVALID_STATE: {
    message: "That can't be done while the document is in its current state. Reload and try again.",
    kind: "conflict",
    retryable: false,
  },
  STALE_VERSION: {
    message: "This draft was changed somewhere else. Reload it to see the latest before continuing.",
    kind: "conflict",
    retryable: false,
  },
  ALREADY_APPROVED: {
    message: "This draft has already been approved into the exam.",
    kind: "conflict",
    retryable: false,
  },
  CONCURRENT_CHANGE: {
    message: "The exam's questions changed at the same moment. Try again.",
    kind: "conflict",
    retryable: true,
  },
  DRAFT_TOO_LARGE: {
    message: "A draft can't hold more questions than the server allows.",
    kind: "conflict",
    retryable: false,
  },
  DRAFT_INCOMPLETE: {
    message: "The draft isn't ready to approve yet. Fix the listed problems and try again.",
    kind: "validation",
    retryable: false,
  },
  EXTRACTION_UNAVAILABLE: {
    message: "Reading exam papers isn't set up on this server. Ask an administrator to enable it.",
    kind: "unavailable",
    retryable: false,
  },
  QUEUE_UNAVAILABLE: {
    message: "Processing couldn't be started right now. Please try again in a moment.",
    kind: "unavailable",
    retryable: true,
  },
  STORAGE_UNAVAILABLE: {
    message: "The file couldn't be stored or read right now. Please try again in a moment.",
    kind: "unavailable",
    retryable: true,
  },
  BUSY: {
    message: "The server is busy reading other papers. Please try again shortly.",
    kind: "unavailable",
    retryable: true,
  },
};

function fieldMap(err: ApiError): Record<string, string> {
  const out: Record<string, string> = {};
  for (const e of err.errors ?? []) {
    if (e.field && !(e.field in out)) out[e.field] = e.error;
  }
  return out;
}

/** Turns anything thrown by the document API into something a teacher can act on. */
export function describeDocumentError(err: unknown): DocumentError {
  if (!(err instanceof ApiError)) {
    return {
      kind: "server",
      message: "Something went wrong. Please try again.",
      retryable: true,
      fields: {},
    };
  }
  const fields = fieldMap(err);
  const known = err.code ? BY_CODE[err.code] : undefined;
  if (known) {
    return { kind: known.kind, code: err.code, message: known.message, retryable: known.retryable, fields };
  }
  switch (err.status) {
    case 0:
      return {
        kind: "network",
        message: "Could not reach the server. Check your connection and try again.",
        retryable: true,
        fields,
      };
    case 401:
      return { kind: "auth", message: "Your session has expired. Please sign in again.", retryable: false, fields };
    case 403:
      return { kind: "forbidden", message: "You do not have permission to do this.", retryable: false, fields };
    case 404:
      return { kind: "notFound", message: "This exam or document could not be found.", retryable: false, fields };
    case 429:
      return {
        kind: "rateLimited",
        message: "Too many uploads in a short time. Wait a minute and try again.",
        retryable: true,
        fields,
      };
    case 400:
      // The server writes validation messages for people (override), so they are safe to show.
      return {
        kind: "validation",
        code: err.code,
        message: err.override && err.message ? err.message : "Some of the details aren't valid. Check them and try again.",
        retryable: false,
        fields,
      };
  }
  return {
    kind: "server",
    code: err.code,
    message: "Something went wrong on our side. Please try again.",
    retryable: true,
    fields,
  };
}

/** Plain-language advice for a failed document, keyed by the server's failure code. */
export function failureAdvice(code: string | undefined): string {
  switch (code) {
    case "PROVIDER_RATE_LIMITED":
    case "PROVIDER_UNAVAILABLE":
    case "PROVIDER_TIMEOUT":
    case "INTERRUPTED":
    case "STORAGE_UNAVAILABLE":
    case "QUEUE_UNAVAILABLE":
    case "INTERNAL_ERROR":
      return "This is usually temporary. Try again in a few minutes.";
    case "PROVIDER_AUTH":
    case "MODEL_NOT_FOUND":
    case "PROVIDER_REJECTED":
      return "This is a server setting, not a problem with your file. Ask an administrator to check the configuration.";
    case "INVALID_PDF":
    case "FILE_MISSING":
      return "Upload the paper again. If it keeps failing, try saving or exporting the PDF again first.";
    case "NO_QUESTIONS_FOUND":
      return "No questions were recognised. Check that this is the question paper, or add the questions yourself.";
    case "OUTPUT_TRUNCATED":
    case "DOCUMENT_TOO_LARGE":
      return "The paper may be too long to read in one go. Try uploading it in two parts.";
    case "INVALID_PROVIDER_OUTPUT":
      return "The reading service didn't return a usable answer. Try again; if it repeats, add the questions yourself.";
    default:
      return "Try again. If it keeps failing, add the questions yourself.";
  }
}
