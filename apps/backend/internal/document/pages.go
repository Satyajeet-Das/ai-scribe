package document

import (
	"context"
	"fmt"
	"sort"

	"github.com/Satyajeet-Das/ai-scribe/internal/document/extract"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/pdf"
)

// maxRemovedLinesShown bounds how many header and footer lines are kept per
// page in the page summary.
const maxRemovedLinesShown = 4

// readPaper turns the PDF into what the extractor takes: each page's text in
// reading order, and, for pages a vision model can help with, the page's
// pictures. It also reports what it found about every page, so that anything it
// sets aside (running headers, pictures it could not read, pictures over the
// budget) is visible to the teacher and not silently dropped.
//
// Every parser call goes through the gate, so a handful of large documents
// cannot use up the machine.
func (s *service) readPaper(ctx context.Context, data []byte) (extract.Input, []PageInfo, error) {
	lim := pdf.Limits{MaxBytes: s.opts.MaxBytes, MaxPages: s.opts.MaxPages, Timeout: s.opts.ParseTimeout}

	release, err := s.gate.Acquire(ctx)
	if err != nil {
		return extract.Input{}, nil, err
	}
	texts, err := pdf.ExtractText(ctx, data, pdf.TextLimits{Limits: lim, MaxRunesPerPage: 20000})
	release()
	if err != nil {
		return extract.Input{}, nil, err
	}
	removed := pdf.StripRepeated(texts)

	pages := make([]extract.Page, len(texts))
	infos := make([]PageInfo, len(texts))
	byNumber := make(map[int]int, len(texts))
	for i, t := range texts {
		pages[i] = extract.Page{Number: t.Number, Kind: string(t.Kind), Text: t.Text()}
		infos[i] = PageInfo{Number: t.Number, Kind: string(t.Kind), Chars: t.Chars, Columns: t.Columns}
		byNumber[t.Number] = i
	}
	for _, r := range removed {
		i := byNumber[r.Page]
		if len(infos[i].RemovedLines) < maxRemovedLinesShown {
			infos[i].RemovedLines = append(infos[i].RemovedLines, r.Text)
		}
	}

	if !s.extractor.CanReadImages() {
		for i := range infos {
			if infos[i].Kind == string(pdf.KindImageOnly) {
				infos[i].Note = "This page has no text layer and no vision model is configured, so it was not read."
			}
		}
		return extract.Input{Pages: pages}, infos, nil
	}

	// Pictures: pages with no text first, because they cannot be read any other
	// way, then pages that have text and may hold diagrams. Both are bounded by a
	// page count and a total size.
	order := make([]int, 0, len(pages))
	for i := range pages {
		if pages[i].Kind == string(pdf.KindImageOnly) {
			order = append(order, i)
		}
	}
	for i := range pages {
		if pages[i].Kind != string(pdf.KindImageOnly) {
			order = append(order, i)
		}
	}

	budget := s.opts.MaxVisionBudget
	used := 0
	for start := 0; start < len(order) && used < s.opts.MaxVisionPages && budget > 0; start += 5 {
		end := start + 5
		if end > len(order) {
			end = len(order)
		}
		batch := order[start:end]
		nums := make([]int, len(batch))
		for k, idx := range batch {
			nums[k] = pages[idx].Number
		}
		sort.Ints(nums)

		release, err := s.gate.Acquire(ctx)
		if err != nil {
			return extract.Input{}, nil, err
		}
		imgs, skipped, err := pdf.ExtractImages(ctx, data, nums, pdf.ImageLimits{
			Limits:        pdf.Limits{MaxBytes: lim.MaxBytes, MaxPages: lim.MaxPages, Timeout: 3 * lim.Timeout},
			MaxPerPage:    s.opts.MaxImagesPerPage,
			MaxImageBytes: s.opts.MaxImageBytes,
		})
		release()
		if err != nil {
			return extract.Input{}, nil, err
		}

		for _, num := range nums {
			i := byNumber[num]
			infos[i].ImagesSkipped = skipped[num]
			got := imgs[num]
			if len(got) == 0 {
				if pages[i].Kind == string(pdf.KindImageOnly) {
					infos[i].Note = "This page is a picture, but no picture in a readable format (JPEG or PNG) could be taken from it, so it was not read."
				}
				continue
			}
			size := int64(0)
			for _, g := range got {
				size += int64(len(g.Data))
			}
			if used >= s.opts.MaxVisionPages || size > budget {
				infos[i].Note = fmt.Sprintf("Its pictures were not read: a document may send at most %d pages of pictures to the reader.", s.opts.MaxVisionPages)
				continue
			}
			budget -= size
			used++
			for _, g := range got {
				pages[i].Images = append(pages[i].Images, extract.Image{Format: g.Format, Data: g.Data})
			}
			infos[i].Images = len(got)
			if pages[i].Kind == string(pdf.KindText) {
				pages[i].Kind = extract.KindMixed
				infos[i].Kind = extract.KindMixed
			}
		}
	}
	for i := range infos {
		if infos[i].Kind == string(pdf.KindImageOnly) && infos[i].Images == 0 && infos[i].Note == "" {
			infos[i].Note = "This page was not read."
		}
	}
	return extract.Input{Pages: pages}, infos, nil
}

// failureMessage is the text shown to the teacher for a failure code. It is
// written here, never taken from a provider.
func failureMessage(code string) string {
	switch code {
	case extract.CodeRateLimited:
		return "The reading service is busy right now. Try again in a few minutes."
	case extract.CodeUnavailable:
		return "The reading service could not be reached. Try again later."
	case extract.CodeTimeout:
		return "Reading the document took too long. Try again, or upload fewer pages."
	case extract.CodeAuth:
		return "The reading service refused the server's credentials. Ask an administrator to check the configuration."
	case extract.CodeRejected:
		return "The reading service could not accept this document."
	case extract.CodeModelNotFound:
		return "The configured reading model is no longer available. Ask an administrator to update the configuration."
	case extract.CodeTruncated:
		return "The reading service's answer was too long for one request. Try a document with fewer questions per page."
	case extract.CodeInvalidOutput:
		return "The reading service did not return a usable answer for this document."
	case extract.CodeNothingFound:
		return "No questions could be found in this document."
	case extract.CodeInvalidPDF:
		return "The file could not be read as a PDF."
	case extract.CodeQueueDown:
		return "The document could not be queued for processing. Try again."
	case extract.CodeStorageDown:
		return "The stored file could not be read right now. Try again later."
	case extract.CodeTooManyBatches:
		return "This document is too large to process in one go."
	case codeFileMissing:
		return "The stored file is missing. Upload the document again."
	case codeInterrupted:
		return "Processing was interrupted. Try again."
	}
	return "Something went wrong while processing this document."
}

// Codes of failures that do not come from the extraction pipeline.
const (
	codeFileMissing = "FILE_MISSING"
	codeInterrupted = "INTERRUPTED"
)
