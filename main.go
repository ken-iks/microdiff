package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"small-go/db"
	"small-go/vid"
	"strconv"

	"github.com/joho/godotenv"
	"google.golang.org/genai"
)

var ctx = context.Background()

func main() {
	// Configure slog to show file and line numbers
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: true, // This adds file:line info
	}))
	slog.SetDefault(logger)

	err := godotenv.Load()
	if err != nil {
		slog.Error("Error loading .env file", "error", err)
		os.Exit(1)
	}
	storage, err := db.NewStorage(ctx, os.Getenv("GCP_PROJ_ID"))
	if err != nil {
		slog.Error("Failed to create storage", "error", err)
		os.Exit(1)
	}
	defer storage.Client.Close()

	db, err := db.GetOrCreateDB("frames.db")
	if err != nil {
		slog.Error("Failed to get/create database", "error", err)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "upload":
		slog.Info("Starting video upload", "video_path", os.Args[2])
		videoID, err := vid.Split(db, ctx, storage, os.Args[2])
		if err != nil {
			slog.Error("Video upload failed", "error", err)
			os.Exit(1)
		}
		slog.Info("Video uploaded successfully", "video_id", videoID)
		fmt.Println("Video uploaded with ID: ", videoID)
	case "edit":
		slog.Info("Starting video edit", "video_id", os.Args[2], "prompt", os.Args[3])
		startTimeSeconds, err := strconv.ParseUint(os.Args[4], 10, 64)
		if err != nil {
			slog.Error("Failed to parse start time", "error", err, "value", os.Args[4])
			os.Exit(1)
		}
		endTimeSeconds, err := strconv.ParseUint(os.Args[5], 10, 64)
		if err != nil {
			slog.Error("Failed to parse end time", "error", err, "value", os.Args[5])
			os.Exit(1)
		}
		vertex, err := genai.NewClient(ctx, &genai.ClientConfig{
			Project:  os.Getenv("GCP_PROJ_ID"),
			Location: os.Getenv("CLOUD_LOCATION_G"), // note: gemini preview models need to be global region not us-central1
			Backend:  genai.BackendVertexAI,
		})
		if err != nil {
			slog.Error("Failed to create genai client", "error", err)
			os.Exit(1)
		}
		err = vid.Edit(db, ctx, storage, vertex, os.Args[2], os.Args[3], startTimeSeconds*1000, endTimeSeconds*1000)
		if err != nil {
			slog.Error("Video edit failed", "error", err)
			os.Exit(1)
		}
		slog.Info("Video edited successfully")
		fmt.Println("Video edited. Check edited/ folder for images")
	default:
		slog.Error("Invalid command", "command", os.Args[1])
		os.Exit(1)
	}
}
