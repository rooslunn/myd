package main

import (
	"crypto/md5"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"

	"github.com/rooslunn/myd/youtube"
)

const (
	BIN_BASH = "/usr/bin/bash"
)

const (
	E_NOT_ALL_ARGS = iota + 1
	E_EXTRACT_VID
	E_VIDEO
	E_COMBINE
)

func main() {
	
	// [ ] todo: combine video and audio
	// [ ] todo: global log
	// [ ] todo: show progress while downloading

	if len(os.Args) < 3 {
		fmt.Println("Usage: go run main.go <youtube_url> <output_file>")
		os.Exit(E_NOT_ALL_ARGS)
	}

	videoURL := os.Args[1]
	outputFile := os.Args[2]

	log := setupLogger()

	videoID, err := youtube.ExtractVideoID(videoURL)
	if err != nil {
		fmt.Println("Can't get video id from url")
		os.Exit(E_EXTRACT_VID)
	}

	client := youtube.NewClient()
	video, err := client.GetVideo(videoID)
	videoUniqueName := titleMD5(video.Title)
	if err != nil {
		log.Error(err.Error())
		os.Exit(E_VIDEO)
	}

	log.Info("Comms clear", "title", video.Title, "videoID", videoID)

	videoFormat := askForFormat(video.Formats.VideoFormats()) 
	audioFormat := askForFormat(video.Formats.AudioFormats()) 

	tempVideoFile := fmt.Sprintf("%s_v_%s", videoUniqueName, outputFile)
	tempAudioFile := fmt.Sprintf("%s_a_%s", videoUniqueName, outputFile)

	err = client.DownloadFormat(video, videoFormat, tempVideoFile, log)
	err = client.DownloadFormat(video, audioFormat, tempAudioFile, log)

	log.Info("Combining video and audio", "video", tempVideoFile, "audio", tempAudioFile)

	cmd := exec.Command(BIN_BASH, "combine.sh", tempVideoFile, tempAudioFile, outputFile)
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		log.Error(err.Error())
		os.Exit(E_COMBINE)
	}

	log.Info("All operations completed. Godshow.")
}

func setupLogger() *slog.Logger {
	// return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func askForFormat(formats []youtube.Format) (youtube.Format) {
	for i, v := range formats {
		fmt.Printf("%d. %s (%s) %s \n", i+1, v.Quality, v.MimeType, youtube.FormatBytes(v.ContentLength))
	}
	fmt.Println()
	fmt.Println("Choose format to download: ")
	
	var formatId uint8
	fmt.Scanln(&formatId)

	return formats[formatId-1]
}

func titleMD5(title string) string {
	h := md5.New()
	io.WriteString(h, title)
	return fmt.Sprintf("%x", h.Sum(nil))
}