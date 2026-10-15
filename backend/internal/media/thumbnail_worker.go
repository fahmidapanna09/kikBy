package media

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif" // decoder registered via side-effect import (animated GIFs: first frame becomes the thumbnail)
	"image/jpeg"
	_ "image/png" // decoder registered via side-effect import; PNG re-encoded as JPEG below for a uniform thumbnail format
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/image/draw"
)

const (
	thumbnailMaxDimension = 320
	thumbnailJPEGQuality  = 80
)

// ThumbnailWorker polls for uploaded images missing a thumbnail, downsizes
// them, and stores the result as a separate object. Video/audio/document
// thumbnails (a poster frame, a waveform, a file-type icon) are a future
// extension of this same pattern — this worker only handles the image kind
// in Phase 3, which is what "thumbnail generation" covers for the message
// types shipped so far.
type ThumbnailWorker struct {
	repo *Repository
	s3   *S3Client
	log  *zap.Logger

	pollInterval time.Duration
	batchSize    int
}

func NewThumbnailWorker(repo *Repository, s3client *S3Client, log *zap.Logger) *ThumbnailWorker {
	return &ThumbnailWorker{repo: repo, s3: s3client, log: log, pollInterval: 3 * time.Second, batchSize: 10}
}

func (w *ThumbnailWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processBatch(ctx)
		}
	}
}

func (w *ThumbnailWorker) processBatch(ctx context.Context) {
	items, err := w.repo.PendingThumbnails(ctx, w.batchSize)
	if err != nil {
		w.log.Error("thumbnail worker: failed to list pending", zap.Error(err))
		return
	}
	for _, m := range items {
		if err := w.processOne(ctx, m); err != nil {
			w.log.Warn("thumbnail worker: failed to process item, will retry next poll", zap.Error(err), zap.String("media_id", m.ID))
		}
	}
}

func (w *ThumbnailWorker) processOne(ctx context.Context, m MediaFile) error {
	data, err := w.s3.GetObjectBytes(ctx, m.ObjectKey)
	if err != nil {
		return fmt.Errorf("download original: %w", err)
	}

	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode image (format guess %q): %w", format, err)
	}

	bounds := img.Bounds()
	origW, origH := bounds.Dx(), bounds.Dy()
	thumbW, thumbH := scaledDimensions(origW, origH, thumbnailMaxDimension)

	dst := image.NewRGBA(image.Rect(0, 0, thumbW, thumbH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: thumbnailJPEGQuality}); err != nil {
		return fmt.Errorf("encode thumbnail: %w", err)
	}

	thumbKey := thumbnailKeyFor(m.ObjectKey)
	if err := w.s3.PutObjectBytes(ctx, thumbKey, "image/jpeg", buf.Bytes()); err != nil {
		return fmt.Errorf("upload thumbnail: %w", err)
	}

	if err := w.repo.SetDimensions(ctx, m.ID, &origW, &origH, nil); err != nil {
		return fmt.Errorf("save dimensions: %w", err)
	}
	if err := w.repo.SetThumbnail(ctx, m.ID, thumbKey); err != nil {
		return fmt.Errorf("save thumbnail key: %w", err)
	}
	return nil
}

func scaledDimensions(w, h, maxDim int) (int, int) {
	if w <= maxDim && h <= maxDim {
		return w, h
	}
	if w >= h {
		newW := maxDim
		newH := h * maxDim / w
		return newW, maxInt(1, newH)
	}
	newH := maxDim
	newW := w * maxDim / h
	return maxInt(1, newW), newH
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func thumbnailKeyFor(objectKey string) string {
	// media/<user>/<uuid>.jpg -> media/<user>/<uuid>_thumb.jpg
	if idx := strings.LastIndex(objectKey, "."); idx != -1 {
		return objectKey[:idx] + "_thumb.jpg"
	}
	return objectKey + "_thumb.jpg"
}
