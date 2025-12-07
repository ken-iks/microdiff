package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"small-go/db"
	"small-go/vid"
	"google.golang.org/genai"
	"strconv"
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

	switch os.Args[1] {
		case "upload":
			err := vid.Split(db, ctx, storage, os.Args[2])
			if err != nil {
				log.Fatal(err)
			}
		case "edit":
			startTimeMicros, err := strconv.ParseUint(os.Args[4], 10, 64)
			if err != nil {
				log.Fatal(err)
			}
			endTimeMicros, err := strconv.ParseUint(os.Args[5], 10, 64)
			if err != nil {
				log.Fatal(err)
			}
			vertex, err := genai.NewClient(ctx, &genai.ClientConfig{
				Project: os.Getenv("GCP_PROJ_ID"),
				Location: os.Getenv("GCP_LOCATION"),
				Backend: genai.BackendVertexAI,
			})
			path, err := vid.Edit(db, ctx, storage, vertex, os.Args[2], os.Args[3], startTimeMicros, endTimeMicros)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("Edited video saved to %s\n", path)
		default:
			log.Fatal("Invalid command")
	}
}
