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
	COMBINE_CMD = "combine.sh"
)

const (
	E_NOT_ALL_ARGS = iota + 1
	E_EXTRACT_VID
	E_VIDEO
	E_COMBINE
)

func main() {

	if len(os.Args) < 4 {
		fmt.Println("Usage: myd <client_type=[ios, android, web]> <youtube_url> <output_file>")
		fmt.Println("Example: ./myd ios 'https://www.youtube.com/watch?v=1vRto-2MMZo' exercise_everyday.mp4")
		os.Exit(E_NOT_ALL_ARGS)
	}

	clientType := os.Args[1]
	videoURL := os.Args[2]
	outputFile := os.Args[3]

	log := setupLogger()

	videoID, err := youtube.ExtractVideoID(videoURL)
	if err != nil {
		fmt.Println("Can't get video id from url")
		os.Exit(E_EXTRACT_VID)
	}

	var clientInfo *youtube.ClientInfo

	switch clientType {
	case "ios":
		clientInfo = &youtube.IOSClient
	case "android":
		clientInfo = &youtube.AndroidClient
	default:
		clientInfo = &youtube.IOSClient
		clientInfo.RandomizeUserAgent()
	}

	client := youtube.NewClient(clientInfo)

	video, err := client.GetVideo(videoID)
	if err != nil {
		log.Error(err.Error())
		os.Exit(E_VIDEO)
	}

	log.Info("Got video info", "title", video.Title, "videoID", videoID, "client", client.Info)

	videoFormat := chooseFormatFrom(video.Formats.VideoFormats()) 
	audioFormat := chooseFormatFrom(video.Formats.AudioFormats()) 

	videoUniqueName := titleMD5(video.Title)
	tempVideoFile := fmt.Sprintf("%s_v_%s", videoUniqueName, outputFile)
	tempAudioFile := fmt.Sprintf("%s_a_%s", videoUniqueName, outputFile)

	ch := make(chan bool)

	downloads := map[string]youtube.Format{
		tempVideoFile: videoFormat,
		tempAudioFile: audioFormat,
	}

	for tempFile, format := range downloads {
		go func() {
			err = client.DownloadFormat(video, format, tempFile, log)
			ch <- true
		}()
	}

	for range len(downloads) {
		<-ch
	}

	log.Info("Combining video and audio", "video", tempVideoFile, "audio", tempAudioFile)

	cmd := exec.Command(BIN_BASH, COMBINE_CMD, tempVideoFile, tempAudioFile, outputFile)
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		log.Error(err.Error())
		os.Exit(E_COMBINE)
	}

	log.Info("Removing temp files", "video", tempVideoFile, "audio", tempAudioFile)
	removeFiles(tempAudioFile, tempVideoFile)

	log.Info("All operations completed. Godshow.")
}

func setupLogger() *slog.Logger {
	// return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func chooseFormatFrom(formats []youtube.Format) (youtube.Format) {
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

func removeFiles(files ...string) error {
	for _, f := range files {
		err := os.Remove(f)
		if err != nil {
			return fmt.Errorf("error removing file %s: %v", f, err)
		}
	}

	return nil
}