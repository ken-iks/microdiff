package db

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Frame struct {
	ID              uint   `gorm:"primaryKey"`
	VideoID         string `gorm:"index:idx_videos"`
	FrameIndex      uint   `gorm:"index:idx_videos"`
	TimestampMicros uint64
	ObjectPath      string
}

func GetOrCreateDB(dbPath string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	err = db.AutoMigrate(&Frame{})
	if err != nil {
		return nil, err
	}
	return db, nil
}

func GetFramesBetween(dbConn *gorm.DB, videoID string, startTimeMicros uint64, endTimeMicros uint64) ([]Frame, error) {
	var frames []Frame
	err := dbConn.Where("video_id = ? AND timestamp_micros BETWEEN ? AND ?", videoID, startTimeMicros, endTimeMicros).Find(&frames).Error
	if err != nil {
		return nil, err
	}
	return frames, nil
}