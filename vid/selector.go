package vid

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // register JPEG decoder for image.DecodeConfig
	"io"
	"log/slog"
	"os"
	"small-go/db"
	"sync"

	"google.golang.org/genai"
)

func imageSize(path string) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

func splitImage(path string, width int, height int, storage *db.Storage, ctx context.Context) []string {
	// returns a list of 9 gcs uris (3x3 grid)
	slog.Info("splitImage: starting", "path", path, "dimensions", fmt.Sprintf("%dx%d", width, height))

	rects := make([]Rect, 9)
	for i := 0; i < 9; i++ {
		rects[i] = Rect{
			MinX:   (i % 3) * width / 3,
			MinY:   (i / 3) * height / 3,
			DeltaX: width / 3,
			DeltaY: height / 3,
		}
	}

	ch := make([]string, 9)
	var wg sync.WaitGroup
	for i := 0; i < 9; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rect := rects[i]
			fp, err := rect.Crop(path, storage, ctx)
			if err != nil {
				slog.Error("splitImage: crop failed", "index", i, "error", err, "rect", fmt.Sprintf("(%d,%d) %dx%d", rect.MinX, rect.MinY, rect.DeltaX, rect.DeltaY))
				ch[i] = ""
				return
			}
			if fp == "" {
				slog.Error("splitImage: crop returned empty URI", "index", i)
				ch[i] = ""
				return
			}
			slog.Debug("splitImage: crop succeeded", "index", i, "uri", fp)
			ch[i] = fp
		}(i)
	}
	wg.Wait()

	// Check for empty URIs
	emptyCount := 0
	for i, uri := range ch {
		if uri == "" {
			emptyCount++
			slog.Warn("splitImage: empty URI found", "index", i)
		}
	}
	if emptyCount > 0 {
		slog.Error("splitImage: some crops failed", "empty_count", emptyCount, "total", 9)
	} else {
		slog.Info("splitImage: all crops succeeded")
	}
	return ch
}

type SelectImageRequest struct {
	SelectedIndex int `json:"selectedIndex"`
}

func Selector(
	ctx context.Context,
	storage *db.Storage,
	vertex *genai.Client,
	prompt string,
	videoPath string,
) (string, error) {
	// returns uri of the section of the image that needs to be edited
	fd, err := storage.Client.Bucket(storage.Bucket).Object(videoPath).NewReader(ctx)
	if err != nil {
		return "", err
	}
	defer fd.Close()

	tmp, err := os.CreateTemp("", "img-*")
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	_, err = io.Copy(tmp, fd)
	if err != nil {
		return "", err
	}

	width, height, err := imageSize(tmp.Name())
	if err != nil {
		return "", err
	}

	uris := splitImage(tmp.Name(), width, height, storage, ctx)

	selectorInstructions, err := os.ReadFile("prompts/selector.md")
	if err != nil {
		return "", err
	}

	contentCfg := genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{
				{Text: string(selectorInstructions)},
			},
		},
		ResponseMIMEType: "application/json",
		ResponseSchema: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"selectedIndex": {
					Type: "integer",
				},
			},
			Required: []string{"selectedIndex"},
		},
	}

	contentBuffer := make([]*genai.Content, 2)
	contentBuffer[0] = &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			{Text: prompt},
		},
	}

	partsBuffer := make([]*genai.Part, len(uris))
	for i, uri := range uris {
		if uri == "" {
			return "", fmt.Errorf("no uri for image %d", i)
		}
		partsBuffer[i] = &genai.Part{
			FileData: &genai.FileData{FileURI: uri, MIMEType: "image/jpeg"},
		}
	}
	contentBuffer[1] = &genai.Content{
		Role:  "user",
		Parts: partsBuffer,
	}

	response, err := vertex.Models.GenerateContent(
		ctx,
		"gemini-2.5-flash",
		contentBuffer,
		&contentCfg,
	)
	if err != nil {
		return "", err
	}
	var request SelectImageRequest
	if txt := response.Text(); txt != "" {
		_ = json.Unmarshal([]byte(txt), &request)
	}
	return uris[request.SelectedIndex], nil
}
