package main

import (
	"io"
	"log/slog"
	"os"

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

	formats := v.Formats.WithAudioChannels() // only get videos with audio
	log.Info("Getting raw stream...")
	stream, _, err := client.GetStream(v, &formats[0])
	if err != nil {
		panic(err)
	}
	defer stream.Close()

	file, err := os.Create("video.mp4")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	log.Info("Coping stream to file...")
	_, err = io.Copy(file, stream)
	if err != nil {
		panic(err)
	}

	log.Info("Video downloaded")
}

func setupLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
}