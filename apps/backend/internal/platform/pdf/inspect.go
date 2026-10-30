// Package pdf is the application's only contact with PDF parsers. Everything an
// uploaded file can do to a parser (crash it, make it loop, make it allocate)
// is contained here: each call runs under recover(), a deadline and, through
// Gate, a limit on how many run at once. Callers get plain Go values and the
// errors below, and never a parser type.
package pdf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

var (
	// ErrNotPDF means the bytes do not look like a PDF at all.
	ErrNotPDF = errors.New("pdf: not a PDF file")
	// ErrMalformed means the file claims to be a PDF but cannot be read as one.
	ErrMalformed = errors.New("pdf: malformed or unreadable PDF")
	// ErrEncrypted means the file is password protected. Encrypted files are
	// refused: nothing here asks for or stores a password.
	ErrEncrypted = errors.New("pdf: encrypted PDF")
	// ErrTooManyPages means the file has more pages than allowed.
	ErrTooManyPages = errors.New("pdf: too many pages")
	// ErrNoPages means the file has no pages.
	ErrNoPages = errors.New("pdf: no pages")
	// ErrTooLarge means the file is bigger than allowed.
	ErrTooLarge = errors.New("pdf: file too large")
	// ErrBusy means the parser limit was reached and the caller gave up waiting.
	ErrBusy = errors.New("pdf: parser busy")
)

// Limits bound what one file may cost.
type Limits struct {
	MaxBytes int64
	MaxPages int
	// Timeout bounds one parser call.
	Timeout time.Duration
}

func (l Limits) timeout() time.Duration {
	if l.Timeout <= 0 {
		return 30 * time.Second
	}
	return l.Timeout
}

// Info is what is known about a valid PDF.
type Info struct {
	PageCount int
}

// Gate limits how many parser calls run at once, so many uploads or jobs at the
// same time cannot use all the CPU and memory. The zero value is not usable.
type Gate struct{ slots chan struct{} }

// NewGate allows n calls at once (at least 1).
func NewGate(n int) *Gate {
	if n < 1 {
		n = 1
	}
	return &Gate{slots: make(chan struct{}, n)}
}

// Acquire waits for a slot, or returns ErrBusy when ctx ends first. The returned
// function releases the slot.
func (g *Gate) Acquire(ctx context.Context) (func(), error) {
	if g == nil {
		return func() {}, nil
	}
	select {
	case g.slots <- struct{}{}:
		return func() { <-g.slots }, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: %v", ErrBusy, ctx.Err())
	}
}

// run calls fn on its own goroutine so a parser that loops or panics cannot take
// the caller with it. A panic is reported as ErrMalformed. When the deadline
// passes the caller is released at once; the goroutine ends when the parser
// does, which is why callers hold a Gate slot until run returns AND the work is
// bounded by the file-size limit.
func run[T any](ctx context.Context, timeout time.Duration, fn func() (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type result struct {
		v   T
		err error
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				var zero T
				done <- result{zero, fmt.Errorf("%w: parser failure", ErrMalformed)}
			}
		}()
		v, err := fn()
		done <- result{v, err}
	}()

	select {
	case r := <-done:
		return r.v, r.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

// CheckSignature does the cheap checks that need no parser: the size limit, the
// %PDF- header, an end-of-file marker near the end and the sniffed type. It
// rejects most non-PDF uploads (renamed images, HTML, executables) before any
// parser sees them.
func CheckSignature(data []byte, maxBytes int64) error {
	if len(data) == 0 {
		return ErrNotPDF
	}
	if maxBytes > 0 && int64(len(data)) > maxBytes {
		return ErrTooLarge
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return ErrNotPDF
	}
	if http.DetectContentType(data) != "application/pdf" {
		return ErrNotPDF
	}
	tail := data
	if len(tail) > 2048 {
		tail = tail[len(tail)-2048:]
	}
	if !bytes.Contains(tail, []byte("%%EOF")) {
		// A truncated upload: the end of the file never arrived.
		return fmt.Errorf("%w: missing end-of-file marker", ErrMalformed)
	}
	return nil
}

func newConfig() *model.Configuration {
	conf := model.NewDefaultConfiguration()
	conf.Offline = true
	conf.ValidationMode = model.ValidationRelaxed
	return conf
}

// Inspect checks that data is a readable, unencrypted PDF within the limits and
// reports its page count.
func Inspect(ctx context.Context, data []byte, lim Limits) (Info, error) {
	if err := CheckSignature(data, lim.MaxBytes); err != nil {
		return Info{}, err
	}
	return run(ctx, lim.timeout(), func() (Info, error) {
		pdfCtx, err := api.ReadContext(bytes.NewReader(data), newConfig())
		if err != nil {
			return Info{}, classify(err)
		}
		if pdfCtx.Encrypt != nil {
			return Info{}, ErrEncrypted
		}
		if err := api.ValidateContext(pdfCtx); err != nil {
			return Info{}, classify(err)
		}
		n := pdfCtx.PageCount
		switch {
		case n <= 0:
			return Info{}, ErrNoPages
		case lim.MaxPages > 0 && n > lim.MaxPages:
			return Info{}, fmt.Errorf("%w: %d pages, limit %d", ErrTooManyPages, n, lim.MaxPages)
		}
		return Info{PageCount: n}, nil
	})
}

// classify turns a parser error into this package's. The parser's own message
// can quote file content, so it is not passed on.
func classify(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	for _, needle := range []string{"password", "encrypt", "Encrypt"} {
		if bytes.Contains([]byte(msg), []byte(needle)) {
			return ErrEncrypted
		}
	}
	return ErrMalformed
}
