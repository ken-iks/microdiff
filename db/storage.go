package db

import (
	"cloud.google.com/go/storage"
	"context"
)

type Storage struct {
	Client *storage.Client
	Bucket string
}

func NewStorage(ctx context.Context, projectID string) (*Storage, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	return &Storage{Client: client, Bucket: "vedit-v0"}, nil
}
