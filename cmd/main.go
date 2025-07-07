package main

import (
	"fmt"
	// "io"
	"log/slog"
	"os"

	"github.com/rooslunn/myd/youtube"
)


func main() {
	
	// [ ] todo: combine video and audio
	// [ ] todo: global log
	// [ ] todo: show progress while downloading

	if len(os.Args) < 3 {
		fmt.Println("Usage: go run main.go <youtube_url> <output_file>")
		os.Exit(1)
	}

	videoURL := os.Args[1]
	// outputFile := os.Args[2]

	log := setupLogger()

	videoID, err := youtube.ExtractVideoID(videoURL)
	if err != nil {
		fmt.Println("Can't get video id from url")
		os.Exit(2)
	}

	client := youtube.NewClient()
	video, err := client.GetVideo(videoID)
	if err != nil {
		log.Error(err.Error())
	}

	log.Info("Comms clear", "title", video.Title, "videoID", videoID)

	videoFormats := video.Formats.VideoFormats()
	audioFormats := video.Formats.AudioFormats()
	videoFormatId := askForFormat(videoFormats) 
	audioFormatId := askForFormat(audioFormats) 
	videoFormat := videoFormats[videoFormatId]
	audioFormat := audioFormats[audioFormatId]

	err = client.DownloadFormat(video, videoFormat, "video.mp4", log)
	err = client.DownloadFormat(video, audioFormat, "audio.mp4", log)

	log.Info("App operations completed. Godshow.")
}

func setupLogger() *slog.Logger {
	// return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func askForFormat(formats []youtube.Format) (uint8) {
	for i, v := range formats {
		fmt.Printf("%d. %s (%s) %s \n", i+1, v.Quality, v.MimeType, youtube.FormatBytes(v.ContentLength))
	}
	fmt.Println()
	fmt.Println("Choose format to download: ")
	
	var formatId uint8
	fmt.Scanln(&formatId)

	return formatId
}

func downloadFormat(format youtube.Format) {

}