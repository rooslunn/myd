package main

import (
	"os"
	"log/slog"
	"github.com/davecgh/go-spew/spew"
)

import (
	"github.com/rooslunn/myd/youtube"
)


func main() {
	log := setupLogger()

	videoID := "7sg9WxMtX9w"
	client := youtube.NewClient()

	log.Info("All comms clear", videoID, client.Info)

	v, err := client.GetVideo(videoID)
	if err != nil {
		log.Error(err.Error())
	}

	log.Info("Video downloaded", "VideoInfo", spew.Sdump(*v),)
}

func setupLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
}