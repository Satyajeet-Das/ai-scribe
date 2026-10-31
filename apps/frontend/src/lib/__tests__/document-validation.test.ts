import { describe, it, expect, afterEach, vi } from "vitest";
import { checkDocumentFile, formatBytes, maxDocumentBytes } from "../document-validation";

const MB = 1024 * 1024;
const file = (name: string, type: string, size: number) => ({ name, type, size });

describe("checkDocumentFile", () => {
  afterEach(() => vi.unstubAllEnvs());

  it("accepts a PDF within the limit", () => {
    expect(checkDocumentFile(file("Paper.pdf", "application/pdf", 3 * MB)).ok).toBe(true);
  });

  it("accepts a PDF whose type the browser left empty, and any letter case", () => {
    expect(checkDocumentFile(file("PAPER.PDF", "", MB)).ok).toBe(true);
  });

  it.each([
    ["a Word file", file("paper.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", MB)],
    ["an image", file("scan.png", "image/png", MB)],
    ["a PDF name on a non-PDF type", file("paper.pdf", "text/html", MB)],
    ["a PDF type on another name", file("paper.txt", "application/pdf", MB)],
    ["no extension", file("paper", "application/pdf", MB)],
    ["a double extension", file("paper.pdf.exe", "application/pdf", MB)],
  ])("rejects %s", (_name, f) => {
    const check = checkDocumentFile(f);
    expect(check.ok).toBe(false);
    expect(check.problem).toBe("not-pdf");
    expect(check.message).toMatch(/PDF/);
  });

  it("rejects an empty file", () => {
    expect(checkDocumentFile(file("a.pdf", "application/pdf", 0)).problem).toBe("empty");
  });

  it("rejects a file over the limit and says both sizes", () => {
    const check = checkDocumentFile(file("big.pdf", "application/pdf", 25 * MB), 20 * MB);
    expect(check.problem).toBe("too-large");
    expect(check.message).toContain("25.0 MB");
    expect(check.message).toContain("20.0 MB");
  });

  it("accepts a file of exactly the limit", () => {
    expect(checkDocumentFile(file("a.pdf", "application/pdf", 20 * MB), 20 * MB).ok).toBe(true);
  });
});

describe("maxDocumentBytes", () => {
  afterEach(() => vi.unstubAllEnvs());

  it("defaults to the server's default of 20 MB", () => {
    expect(maxDocumentBytes()).toBe(20 * MB);
  });

  it("follows NEXT_PUBLIC_DOCUMENT_MAX_MB, and ignores nonsense", () => {
    vi.stubEnv("NEXT_PUBLIC_DOCUMENT_MAX_MB", "8");
    expect(maxDocumentBytes()).toBe(8 * MB);
    vi.stubEnv("NEXT_PUBLIC_DOCUMENT_MAX_MB", "abc");
    expect(maxDocumentBytes()).toBe(20 * MB);
    vi.stubEnv("NEXT_PUBLIC_DOCUMENT_MAX_MB", "-3");
    expect(maxDocumentBytes()).toBe(20 * MB);
  });
});

describe("formatBytes", () => {
  it("uses a sensible unit", () => {
    expect(formatBytes(512)).toBe("512 bytes");
    expect(formatBytes(2048)).toBe("2.0 KB");
    expect(formatBytes(5 * MB)).toBe("5.0 MB");
  });
});
