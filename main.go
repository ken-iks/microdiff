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
	"github.com/joho/godotenv"
)

var ctx = context.Background()

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
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
			videoID, err := vid.Split(db, ctx, storage, os.Args[2])
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println("Video uploaded with ID: ", videoID)
		case "edit":
			startTimeSeconds, err := strconv.ParseUint(os.Args[4], 10, 64)
			if err != nil {
				log.Fatal(err)
			}
			endTimeSeconds, err := strconv.ParseUint(os.Args[5], 10, 64)
			if err != nil {
				log.Fatal(err)
			}
			vertex, err := genai.NewClient(ctx, &genai.ClientConfig{
				Project: os.Getenv("GCP_PROJ_ID"),
				Location: os.Getenv("CLOUD_LOCATION_G"),
				Backend: genai.BackendVertexAI,
			})
			err = vid.Edit(db, ctx, storage, vertex, os.Args[2], os.Args[3], startTimeSeconds * 1000, endTimeSeconds * 1000)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println("Video edited. Check edited/ folder for images")
		default:
			log.Fatal("Invalid command")
	}
}
