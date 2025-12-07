package vid

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"gocv.io/x/gocv"
	"gorm.io/gorm"
	"small-go/db"
	"time"
)

func uploadFrame(
	dbConn *gorm.DB, 
	ctx context.Context, 
	storage *db.Storage, 
	imageBytes []byte, 
	videoID string, 
	frameIndex uint, 
	tsMicros uint64,
) error {
	var objectPath string = fmt.Sprintf("%s/frame_%4d.jpg", videoID, frameIndex)
	ctx, cancel := context.WithTimeout(ctx, time.Second*50)
	defer cancel()
	wc := storage.Client.Bucket(storage.Bucket).Object(objectPath).NewWriter(ctx)
	_, err := wc.Write(imageBytes)
	if err != nil {
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}
	frame := db.Frame{
		VideoID:         videoID,
		FrameIndex:      frameIndex,
		TimestampMicros: tsMicros,
		ObjectPath:      objectPath,
	}
	err = dbConn.Create(&frame).Error
	if err != nil {
		return err
	}
	return nil
}

func Split(
	dbConn *gorm.DB, 
	ctx context.Context, 
	storage *db.Storage, 
	videoPath string,
) error {
	video, err := gocv.VideoCaptureFile(videoPath)
	if err != nil {
		return err
	}
	defer video.Close()

	videoID := uuid.New().String()

	// traverse
	frame := gocv.NewMat() // storage container for pixel data
	defer frame.Close()

	n := 0 // frame counter
	ch := make(chan error, int(video.Get(gocv.VideoCaptureFrameCount)))
	for {
		if ok := video.Read(&frame); !ok || frame.Empty() {
			break
		}
		n++

		tsMicros := video.Get(gocv.VideoCapturePosMsec)

		buf, err := gocv.IMEncode(".jpg", frame)
		if err != nil {
			return err
		}
		imageBytes := make([]byte, len(buf.GetBytes()))
		//TODO: note that this may be not needed. Check if buf.GetBytes() is a slice and not a pointer.
		copy(imageBytes, buf.GetBytes())
		buf.Close()

		go func(frameIndex uint, imageBytes []byte, tsMicros uint64) {
			ch <- uploadFrame(dbConn, ctx, storage, imageBytes, videoID, frameIndex, tsMicros)
		}(uint(n), imageBytes, uint64(tsMicros))
	}

	for i := 0; i < n; i++ {
		err := <-ch
		if err != nil {
			return err
		}
	}
	return nil
}
