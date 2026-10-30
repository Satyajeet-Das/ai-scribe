package pdf

import (
	"bytes"
	"context"
	"image"
	_ "image/jpeg" // register decoders for DecodeConfig
	_ "image/png"
	"io"
	"sort"
	"strconv"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// Image is a picture embedded in a page, in a format a vision model accepts.
type Image struct {
	Page   int
	Width  int
	Height int
	// Format is "jpg" or "png".
	Format string
	Data   []byte
}

// ImageLimits bound what is pulled out of a file.
type ImageLimits struct {
	Limits
	// MinSide ignores pictures smaller than this on either side (logos, rules)
	// unless a page has nothing else.
	MinSide int
	// MaxPerPage bounds the pictures kept for one page.
	MaxPerPage int
	// MaxImageBytes bounds one picture's size.
	MaxImageBytes int64
}

// imageChunk is how many pages are read in one pass. Each pass parses the whole
// file once, so reading page by page would parse it as often as it has pages;
// reading everything at once would hold every picture of the file at the same
// time. A few pages at a time is the compromise.
const imageChunk = 5

// ExtractImages returns the usable pictures of the given pages (1-based), the
// biggest first. Pictures in formats other than JPEG and PNG (JBIG2, CCITT,
// TIFF and so on) are not returned; they are counted in skipped so the caller
// can say a page could not be read.
func ExtractImages(ctx context.Context, data []byte, pages []int, lim ImageLimits) (images map[int][]Image, skipped map[int]int, err error) {
	if err := CheckSignature(data, lim.Limits.MaxBytes); err != nil {
		return nil, nil, err
	}
	minSide, perPage, maxBytes := lim.MinSide, lim.MaxPerPage, lim.MaxImageBytes
	if minSide <= 0 {
		minSide = 150
	}
	if perPage <= 0 {
		perPage = 3
	}
	if maxBytes <= 0 {
		maxBytes = 4 << 20
	}
	sorted := append([]int(nil), pages...)
	sort.Ints(sorted)

	type out struct {
		images  map[int][]Image
		skipped map[int]int
	}
	res, err := run(ctx, lim.timeout(), func() (out, error) {
		o := out{images: map[int][]Image{}, skipped: map[int]int{}}
		for start := 0; start < len(sorted); start += imageChunk {
			if err := ctx.Err(); err != nil {
				return o, err
			}
			end := start + imageChunk
			if end > len(sorted) {
				end = len(sorted)
			}
			chunk := sorted[start:end]
			sel := make([]string, len(chunk))
			for i, p := range chunk {
				sel[i] = strconv.Itoa(p)
			}
			found, err := api.ExtractImagesRaw(bytes.NewReader(data), sel, newConfig())
			if err != nil {
				return o, classify(err)
			}
			// The parser returns one map per selected page but in no fixed order,
			// so each picture is filed under the page number it carries.
			inChunk := make(map[int]bool, len(chunk))
			for _, p := range chunk {
				inChunk[p] = true
			}
			byPage := map[int][]Image{}
			for _, byObj := range found {
				for _, im := range byObj {
					pg := im.PageNr
					if !inChunk[pg] {
						continue
					}
					format := im.FileType
					if format == "jpeg" {
						format = "jpg"
					}
					if format != "jpg" && format != "png" {
						o.skipped[pg]++
						continue
					}
					b, err := io.ReadAll(io.LimitReader(im, maxBytes+1))
					if err != nil || int64(len(b)) > maxBytes || len(b) == 0 {
						o.skipped[pg]++
						continue
					}
					cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
					if err != nil {
						o.skipped[pg]++
						continue
					}
					byPage[pg] = append(byPage[pg], Image{Page: pg, Width: cfg.Width, Height: cfg.Height, Format: format, Data: b})
				}
			}
			for pg, cands := range byPage {
				sort.SliceStable(cands, func(a, b int) bool {
					return cands[a].Width*cands[a].Height > cands[b].Width*cands[b].Height
				})
				for _, c := range cands {
					if len(o.images[pg]) >= perPage {
						break
					}
					small := c.Width < minSide || c.Height < minSide
					// A page whose only picture is small keeps it: it may be all
					// there is of a scan.
					if small && len(cands) > 1 {
						continue
					}
					o.images[pg] = append(o.images[pg], c)
				}
			}
		}
		return o, nil
	})
	if err != nil {
		return nil, nil, err
	}
	return res.images, res.skipped, nil
}
