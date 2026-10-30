package extract

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Options bound a run.
type Options struct {
	// MaxCharsPerRequest is how much page text goes to the model in one call.
	MaxCharsPerRequest int
	// MaxBatches bounds the calls one document may cost.
	MaxBatches int
	// MaxQuestions bounds the draft.
	MaxQuestions int
}

func (o Options) withDefaults() Options {
	if o.MaxCharsPerRequest <= 0 {
		o.MaxCharsPerRequest = 12000
	}
	if o.MaxBatches <= 0 {
		o.MaxBatches = 120
	}
	if o.MaxQuestions <= 0 {
		o.MaxQuestions = 500
	}
	return o
}

// Extractor runs a whole paper through a Model.
type Extractor struct {
	model Model
	opts  Options
}

func New(model Model, opts Options) *Extractor {
	return &Extractor{model: model, opts: opts.withDefaults()}
}

// Name identifies the model in logs and the document record.
func (e *Extractor) Name() string { return e.model.Name() }

// PromptVersion labels the prompt in use.
func (e *Extractor) PromptVersion() string { return e.model.PromptVersion() }

// CanReadImages reports whether picture pages can be read.
func (e *Extractor) CanReadImages() bool { return e.model.CanReadImages() }

type batch struct {
	req   Request
	pages []int
}

// plan splits the paper into model calls in page order. Pages with a text layer
// go together up to the size limit; a picture page, or a page with pictures when
// a vision model is available, is a call of its own. Pages nothing can read are
// returned as unassigned notes, so the teacher knows they were skipped.
func (e *Extractor) plan(in Input) ([]batch, []Unassigned) {
	var batches []batch
	var notes []Unassigned
	var cur []Page
	size := 0
	flush := func() {
		if len(cur) == 0 {
			return
		}
		nums := make([]int, len(cur))
		for i, p := range cur {
			nums[i] = p.Number
		}
		batches = append(batches, batch{req: Request{Pages: cur}, pages: nums})
		cur, size = nil, 0
	}

	canSee := e.model.CanReadImages()
	for _, p := range in.Pages {
		hasText := strings.TrimSpace(p.Text) != ""
		hasImages := len(p.Images) > 0
		switch {
		case canSee && hasImages && (!hasText || p.Kind == KindMixed):
			flush()
			batches = append(batches, batch{req: Request{Pages: []Page{p}, Vision: true}, pages: []int{p.Number}})
		case !hasText:
			reason := "This page has no text layer and no picture could be read from it."
			if hasImages && !canSee {
				reason = "This page is a picture and no vision model is configured, so it was not read."
			}
			notes = append(notes, Unassigned{Page: p.Number, Text: "", Reason: reason})
		default:
			if hasImages && !canSee && p.Kind == KindMixed {
				notes = append(notes, Unassigned{Page: p.Number, Text: "", Reason: "This page has pictures that were not read (no vision model is configured). Describe any diagram yourself."})
			}
			n := len(p.Text)
			if len(cur) > 0 && size+n > e.opts.MaxCharsPerRequest {
				flush()
			}
			cur = append(cur, p)
			size += n
		}
	}
	flush()
	return batches, notes
}

// Extract reads the whole paper. progress, when not nil, is told how many calls
// are done out of the total. A failure of any call fails the run: a paper with
// a gap in the middle would be worse than no draft.
func (e *Extractor) Extract(ctx context.Context, in Input, progress func(done, total int)) (*Result, error) {
	batches, notes := e.plan(in)
	if len(batches) == 0 {
		return nil, NewError(CodeNothingFound, false, "no page could be read")
	}
	if len(batches) > e.opts.MaxBatches {
		return nil, NewError(CodeTooManyBatches, false, "the document needs %d requests, the limit is %d", len(batches), e.opts.MaxBatches)
	}

	pageText := map[int]string{}
	for _, p := range in.Pages {
		pageText[p.Number] = p.Text
	}

	result := &Result{Questions: []Question{}, Unassigned: append([]Unassigned{}, notes...)}
	var all []batchQuestion
	if progress != nil {
		progress(0, len(batches))
	}
	for i, b := range batches {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		br, err := e.runBatch(ctx, b)
		if err != nil {
			return nil, err
		}
		if result.DocumentInstructions == "" {
			result.DocumentInstructions = br.instructions
		}
		for _, q := range br.questions {
			ground(&q.Question, pageText, b.pages)
			all = append(all, q)
		}
		result.Unassigned = append(result.Unassigned, br.unassigned...)
		if progress != nil {
			progress(i+1, len(batches))
		}
	}

	result.Questions = assemble(all, result)
	if len(result.Questions) > e.opts.MaxQuestions {
		return nil, NewError(CodeInvalidOutput, false, "the draft has more than %d questions", e.opts.MaxQuestions)
	}
	if len(result.Questions) == 0 {
		return nil, NewError(CodeNothingFound, false, "no questions were found")
	}
	return result, nil
}

// runBatch calls the model, and asks once more if its reply cannot be used: a
// model that wandered off the format once usually does not do it twice.
// Transport failures are not retried here; the job's own retry policy handles
// them.
func (e *Extractor) runBatch(ctx context.Context, b batch) (*batchResult, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		reply, err := e.model.Complete(ctx, b.req)
		if err != nil {
			return nil, err
		}
		br, err := parse(reply, b.pages)
		if err == nil {
			return br, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// assemble joins questions that continue across a batch boundary, moves
// questions with no wording to the unassigned list, and flags repeated and
// skipped numbers.
func assemble(in []batchQuestion, res *Result) []Question {
	var out []Question
	for _, bq := range in {
		q := bq.Question
		if bq.continues {
			if n := len(out); n > 0 {
				mergeInto(&out[n-1], q)
				continue
			}
			warn(&q, WarnContinuationOrphan, "This was said to continue an earlier question, but there is none before it.", false)
		}
		if strings.TrimSpace(q.Text) == "" {
			// Keep what was found, in words the teacher can read.
			var b strings.Builder
			if q.NumberLabel != "" {
				b.WriteString(q.NumberLabel + " ")
			}
			for _, o := range q.Options {
				b.WriteString(fmt.Sprintf("%s. %s ", o.Label, o.Text))
			}
			page := 0
			if len(q.SourcePages) > 0 {
				page = q.SourcePages[0]
			}
			res.Unassigned = append(res.Unassigned, Unassigned{
				Page: page, Text: strings.TrimSpace(b.String()), Reason: "A question with no wording was found here.",
			})
			continue
		}
		out = append(out, q)
	}

	seen := map[string]bool{}
	prev := 0
	for i := range out {
		l := strings.TrimSpace(strings.Trim(out[i].NumberLabel, ".):Qq "))
		if l == "" {
			continue
		}
		if seen[l] {
			warn(&out[i], WarnDuplicateNumber, fmt.Sprintf("The number %s is used by more than one question.", l), false)
		}
		seen[l] = true
		if n, err := strconv.Atoi(l); err == nil {
			if prev > 0 && n > prev+1 {
				warn(&out[i], WarnNumberingGap, fmt.Sprintf("The numbers jump from %d to %d. A question may be missing.", prev, n), false)
			}
			prev = n
		}
	}
	return out
}

// mergeInto adds a continuation to the question it continues.
func mergeInto(dst *Question, src Question) {
	if src.Text != "" {
		dst.Text = strings.TrimSpace(dst.Text + "\n" + src.Text)
	}
	have := map[string]bool{}
	for _, o := range dst.Options {
		have[o.Label] = true
	}
	for _, o := range src.Options {
		if have[o.Label] {
			warn(dst, WarnDuplicateOption, fmt.Sprintf("Option %s appears on both pages the question runs across.", o.Label), false)
		}
		have[o.Label] = true
		dst.Options = append(dst.Options, o)
	}
	dst.Tables = append(dst.Tables, src.Tables...)
	dst.Diagrams = append(dst.Diagrams, src.Diagrams...)
	for _, p := range src.SourcePages {
		found := false
		for _, e := range dst.SourcePages {
			found = found || e == p
		}
		if !found {
			dst.SourcePages = append(dst.SourcePages, p)
		}
	}
	dst.Warnings = append(dst.Warnings, src.Warnings...)
	if src.Confidence == ConfidenceLow || (src.Confidence == ConfidenceMedium && dst.Confidence == ConfidenceHigh) {
		dst.Confidence = src.Confidence
	}
	if dst.Type == TypeEssay && len(dst.Options) >= 2 {
		warn(dst, WarnTypeUncertain, "This is marked as a written question but has options. Check its type.", false)
	}
}
