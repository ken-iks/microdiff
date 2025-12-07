package db

import (
	"cloud.google.com/go/storage"
	"context"
	"google.golang.org/api/option"
	"os"
)

type Storage struct {
	Client *storage.Client
}

func NewStorage(ctx context.Context, projectID string) (*Storage, error) {
	client, err := storage.NewClient(ctx, option.WithCredentialsFile(os.Getenv("GCS_CREDS_PATH")))
	if err != nil {
		return nil, err
	}
	return &Storage{Client: client}, nil
}
