package vid

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"small-go/db"
	"google.golang.org/genai"
	"gorm.io/gorm"
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
	startTimeMillis uint64,
	endTimeMillis uint64,
) error {
	frames, err := db.GetFramesBetween(dbConn, videoID, startTimeMillis, endTimeMillis)
	if err != nil {
		return err
	}
	instructions, err := os.ReadFile("vid/instructions.md")
	if err != nil {
		return err
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
		return err
	}
	var requests []EditImageRequest
	if txt := response.Text(); txt != "" {
		_ = json.Unmarshal([]byte(txt), &requests)
	}

	ch := make(chan error, len(requests))
	sem := make(chan struct{}, 10) // bottlneck here is rate limts for api
	os.MkdirAll("edited", 0755)
	for _, request := range requests {
		sem <- struct{}{} // acquire semaphore slot
		go func(frame db.Frame, prompt string) {
			defer func() { <-sem }() // release semaphore slot
			uri := fmt.Sprintf("gs://%s/%s", storage.Bucket, frame.ObjectPath)	
			// image editing happens through the generate content endpoint NOT the edit image endpoint.
			// edit image is not comnpatable with gemini models (only imagen models which are going to be deprecated soon)
			resp, err := vertex.Models.GenerateContent(
				ctx,
				"gemini-2.5-flash-image", // gemini-3-pro-image-preview is HEAVILY rate limited at the moment due to DSQ (12/09/25)
				[]*genai.Content{
					{
						Role: "user",
						Parts: []*genai.Part{
							{Text: prompt},
							{FileData: &genai.FileData{FileURI: uri, MIMEType: "image/jpeg"}},
						},
					},
				},
				&genai.GenerateContentConfig{},
			)
			if err != nil {
				ch <- err
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