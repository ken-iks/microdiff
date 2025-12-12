package vid

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"small-go/db"
	"strings"
	"time"

	"google.golang.org/genai"
	"gorm.io/gorm"
)

type EditImageRequest struct {
	ImageIndex  int    `json:"imageIndex"`
	ImagePrompt string `json:"imagePrompt"`
}

func Edit(
	dbConn *gorm.DB,
	ctx context.Context,
	storage *db.Storage,
	vertex *genai.Client,
	videoID string,
	prompt string,
	startTimeMillis uint64,
	endTimeMillis uint64,
) error {
	frames, err := db.GetFramesBetween(dbConn, videoID, startTimeMillis, endTimeMillis)
	if err != nil {
		return err
	}
	prompterInstructions, err := os.ReadFile("prompts/prompter.md")
	if err != nil {
		return err
	}
	s := string(prompterInstructions)
	contentCfg := genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{
				{Text: s},
			},
		},
		ResponseMIMEType: "application/json",
		ResponseSchema: &genai.Schema{
			Type: "array",
			Items: &genai.Schema{
				Type: "object",
				Properties: map[string]*genai.Schema{
					"imageIndex": {
						Type: "integer",
					},
					"imagePrompt": {
						Type: "string",
					},
				},
				Required: []string{"imageIndex", "imagePrompt"},
			},
		},
	}

	contentBuffer := make([]*genai.Content, len(frames)+1)
	contentBuffer[0] = &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			{Text: prompt},
		},
	}
	for i, frame := range frames {
		contentBuffer[i+1] = &genai.Content{
			Role: "user",
			Parts: []*genai.Part{
				{
					FileData: &genai.FileData{
						FileURI:  fmt.Sprintf("gs://%s/%s", storage.Bucket, frame.ObjectPath),
						MIMEType: "image/jpeg",
					},
				},
			},
		}
	}

	response, err := vertex.Models.GenerateContent(
		ctx,
		"gemini-3-pro-preview",
		contentBuffer,
		&contentCfg,
	)
	if err != nil {
		return err
	}
	var requests []EditImageRequest
	if txt := response.Text(); txt != "" {
		_ = json.Unmarshal([]byte(txt), &requests)
	}

	ch := make(chan error, len(requests))
	sem := make(chan struct{}, 10) // bottlneck here is rate limts for api
	os.MkdirAll("edited", 0755)
	editorInstructions, err := os.ReadFile("prompts/editor.md")
	if err != nil {
		return err
	}
	for _, request := range requests {
		sem <- struct{}{} // acquire semaphore slot
		go func(frame db.Frame, prompt string) {
			defer func() { <-sem }() // release semaphore slot
			// uri := fmt.Sprintf("gs://%s/%s", storage.Bucket, frame.ObjectPath)
			uri, err := Selector(ctx, storage, vertex, prompt, frame.ObjectPath)
			if err != nil {
				ch <- err
				return
			}
			// image editing happens through the generate content endpoint NOT the edit image endpoint.
			// edit image is not comnpatable with gemini models (only imagen models which are going to be deprecated soon)
			// Retry logic for rate-limited gemini-3-pro-image-preview
			var resp *genai.GenerateContentResponse
			maxRetries := 5
			baseDelay := 2 * time.Second
			for attempt := 0; attempt < maxRetries; attempt++ {
				var err error
				resp, err = vertex.Models.GenerateContent(
					ctx,
					"gemini-3-pro-image-preview", // gemini-3-pro-image-preview is HEAVILY rate limited at the moment due to DSQ (12/09/25)
					[]*genai.Content{
						{
							Role: "user",
							Parts: []*genai.Part{
								{Text: prompt},
								{FileData: &genai.FileData{FileURI: uri, MIMEType: "image/jpeg"}},
							},
						},
					},
					&genai.GenerateContentConfig{
						SystemInstruction: &genai.Content{
							Parts: []*genai.Part{
								{Text: string(editorInstructions)},
							},
						},
					},
				)
				if err == nil {
					break
				}

				// Check if it's a 429 error (rate limit)
				errStr := err.Error()
				if strings.Contains(errStr, "429") || strings.Contains(errStr, "RESOURCE_EXHAUSTED") {
					if attempt < maxRetries-1 {
						delay := baseDelay * time.Duration(1<<uint(attempt)) // exponential backoff
						slog.Warn("Rate limit hit, retrying image edit", "attempt", attempt+1, "delay", delay, "frame_index", frame.FrameIndex, "error", errStr)
						time.Sleep(delay)
						continue
					}
				}
				// If not a 429 or we've exhausted retries, return the error
				ch <- err
				return
			}
			if resp == nil {
				ch <- fmt.Errorf("failed to get response after %d attempts", maxRetries)
				return
			}

			for _, part := range resp.Candidates[0].Content.Parts {
				if part.InlineData != nil {
					editedImageBytes := part.InlineData.Data
					err := os.WriteFile(fmt.Sprintf("edited/frame_%04d.jpg", frame.FrameIndex), editedImageBytes, 0644)
					if err != nil {
						ch <- err
						return
					}
					ch <- nil
					return
				}
			}

			fmt.Println("Could not edit image")
			ch <- fmt.Errorf("could not edit image")
		}(frames[request.ImageIndex], request.ImagePrompt)
	}

	for i := 0; i < len(requests); i++ {
		err := <-ch
		if err != nil {
			return err
		}
	}

	return nil
}
