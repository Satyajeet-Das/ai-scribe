/**
 * Client-side checks on a chosen PDF. They exist so a teacher is told at once,
 * before a long upload, that a file cannot work; the server repeats every check
 * and is the authority. A file that passes here can still be refused.
 */

/**
 * The server's default upload limit is 20 MiB, and it is configurable there
 * (SCRIBE_DOCUMENTS.MAX_BYTES) with no endpoint that reports it. Set
 * NEXT_PUBLIC_DOCUMENT_MAX_MB to match a server that was configured differently.
 */
export function maxDocumentBytes(): number {
  const mb = Number(process.env.NEXT_PUBLIC_DOCUMENT_MAX_MB);
  return (Number.isFinite(mb) && mb > 0 ? mb : 20) * 1024 * 1024;
}

export type FileProblem = "none" | "not-pdf" | "empty" | "too-large";

export interface FileCheck {
  ok: boolean;
  problem: FileProblem;
  /** Plain-language message for the teacher; empty when ok. */
  message: string;
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} bytes`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/** Checks the extension, the type the browser reports, and the size. */
export function checkDocumentFile(
  file: Pick<File, "name" | "type" | "size">,
  maxBytes: number = maxDocumentBytes()
): FileCheck {
  const hasPdfName = /\.pdf$/i.test(file.name);
  // Browsers leave the type empty for some files, so an empty type is not a reason to refuse.
  const typeOk = file.type === "" || file.type === "application/pdf";
  if (!hasPdfName || !typeOk) {
    return {
      ok: false,
      problem: "not-pdf",
      message: "Only PDF files can be uploaded. Choose a file that ends in .pdf.",
    };
  }
  if (file.size === 0) {
    return { ok: false, problem: "empty", message: "This file is empty. Choose a different file." };
  }
  if (file.size > maxBytes) {
    return {
      ok: false,
      problem: "too-large",
      message: `This file is ${formatBytes(file.size)}, which is over the ${formatBytes(maxBytes)} limit. Choose a smaller file or split the paper.`,
    };
  }
  return { ok: true, problem: "none", message: "" };
}
