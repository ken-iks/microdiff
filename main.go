package main

import (
	"context"
	"log"
	"os"
	"small-go/db"
	"small-go/vid"
)

var ctx = context.Background()

func main() {
	storage, err := db.NewStorage(ctx, os.Getenv("GCP_PROJ_ID"))
	if err != nil {
		log.Fatal(err)
	}
	defer storage.Client.Close()

	db, err := db.GetOrCreateDB("frames.db")
	if err != nil {
		log.Fatal(err)
	}

	// storage client can then be passed to any func that needs it
	if os.Args[1] == "upload" {
		err = vid.Split(db, ctx, storage, os.Args[2])
		if err != nil {
			log.Fatal(err)
		}
	}
}
