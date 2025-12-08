package vid

import (
	"context"
	"small-go/db"
	"gorm.io/gorm"
	"google.golang.org/genai"
	"fmt"
	"os"
	"encoding/json"
)

type EditImageRequest struct {
	ImageIndex int `json:"imageIndex"`
	ImagePrompt string `json:"imagePrompt"`
}

func Edit(
	dbConn *gorm.DB,
	ctx context.Context,
	storage *db.Storage,
	vertex *genai.Client,
	videoID string,
	prompt string,
	startTimeMicros uint64,
	endTimeMicros uint64,
) (string, error) {
	frames, err := db.GetFramesBetween(dbConn, videoID, startTimeMicros, endTimeMicros)
	if err != nil {
		return "", err
	}
	instructions, err := os.ReadFile("instructions.md")
	if err != nil {
		return "", err
	}
	s := string(instructions)
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

	contentBuffer := make([]*genai.Content, len(frames) + 1)
	contentBuffer[0] = &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			{Text: prompt},
		},
	}
	for i, frame := range frames {
		contentBuffer[i + 1] = &genai.Content{
			Role: "user",
			Parts: []*genai.Part{
				{
					FileData: &genai.FileData{
						FileURI: fmt.Sprintf("gs://%s/%s", storage.Bucket, frame.ObjectPath),
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
		return "", err
	}
	var requests []EditImageRequest
	if txt := response.Text(); txt != "" {
		_ = json.Unmarshal([]byte(txt), &requests)
	}

	ch := make(chan error, len(requests))
	os.MkdirAll("edited", 0755)
	for _, request := range requests {
		go func(frame db.Frame, prompt string) {
			uri := fmt.Sprintf("gs://%s/%s", storage.Bucket, frame.ObjectPath)
			
			// TODO: store image locally so we don't have to download it from GCS every time
			editedImage, err := vertex.Models.EditImage(
				ctx,
				"gemini-3-pro-image-preview",
				prompt,
				[]genai.ReferenceImage{
					&genai.RawReferenceImage{
						ReferenceImage: &genai.Image{
							GCSURI: uri,
						},
					},
				},
				&genai.EditImageConfig{
					NumberOfImages: 1,
				},
			)
			if err != nil {
				ch <- err
				return
			}
			imageBytes := editedImage.GeneratedImages[0].Image.ImageBytes
			os.WriteFile(fmt.Sprintf("edited/frame_%4d.jpg", request.ImageIndex), imageBytes, 0644)
			ch <- nil
		}(frames[request.ImageIndex], request.ImagePrompt)
	}

	for i := 0; i < len(frames); i++ {
		err := <-ch
		if err != nil {
			return "", err
		}
	}

	return "", nil
}