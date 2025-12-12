package vid

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"small-go/db"
	"strconv"
)

type Rect struct {
	MinX   int
	MinY   int
	DeltaX int
	DeltaY int
}

func (r *Rect) Crop(fp string, storage *db.Storage, ctx context.Context) (string, error) {
	slog.Info("Crop: starting", "file", fp, "rect", fmt.Sprintf("(%d,%d) %dx%d", r.MinX, r.MinY, r.DeltaX, r.DeltaY))

	tmp, err := os.CreateTemp("", "crop-*.jpg")
	if err != nil {
		slog.Error("Crop: failed to create temp file", "error", err)
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close() // Close the file handle so vips can write to it
	slog.Debug("Crop: created temp file", "path", tmpPath)

	cmd := exec.Command(
		"vips", "crop",
		fp, tmpPath,
		strconv.Itoa(r.MinX), strconv.Itoa(r.MinY), strconv.Itoa(r.DeltaX), strconv.Itoa(r.DeltaY),
	)
	if err := cmd.Run(); err != nil {
		slog.Error("Crop: vips command failed", "error", err, "command", cmd.String(), "input", fp, "output", tmpPath)
		return "", fmt.Errorf("vips crop failed: %w", err)
	}
	slog.Debug("Crop: vips command succeeded", "output", tmpPath)

	// Reopen the file for reading
	tmpFile, err := os.Open(tmpPath)
	if err != nil {
		slog.Error("Crop: failed to reopen temp file for reading", "error", err, "path", tmpPath)
		return "", fmt.Errorf("reopen temp file: %w", err)
	}
	defer tmpFile.Close()

	// Check file size before uploading
	fileInfo, err := tmpFile.Stat()
	if err != nil {
		slog.Error("Crop: failed to stat temp file", "error", err, "path", tmpPath)
		return "", fmt.Errorf("stat temp file: %w", err)
	}
	slog.Debug("Crop: temp file size", "path", tmpPath, "size", fileInfo.Size())

	if fileInfo.Size() == 0 {
		slog.Error("Crop: temp file is empty after vips crop", "path", tmpPath, "input", fp)
		return "", fmt.Errorf("crop produced empty file")
	}

	w := storage.Client.Bucket(storage.Bucket).Object(tmpPath).NewWriter(ctx)
	copied, err := io.Copy(w, tmpFile)
	if err != nil {
		slog.Error("Crop: failed to copy to GCS", "error", err, "path", tmpPath, "bytes_copied", copied)
		w.Close()
		return "", fmt.Errorf("copy to GCS: %w", err)
	}
	slog.Debug("Crop: copied to GCS", "bytes", copied, "path", tmpPath)

	if err := w.Close(); err != nil {
		slog.Error("Crop: failed to close GCS writer", "error", err, "path", tmpPath)
		return "", fmt.Errorf("close GCS writer: %w", err)
	}

	uri := fmt.Sprintf("gs://%s/%s", storage.Bucket, tmpPath)
	slog.Info("Crop: completed successfully", "uri", uri)
	return uri, nil
}
