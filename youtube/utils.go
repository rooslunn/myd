package youtube

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

type chunk struct {
	start int64
	end   int64
	data  chan []byte
}

func getChunks(totalSize, chunkSize int64) []chunk {
	var chunks []chunk

	for start := int64(0); start < totalSize; start += chunkSize {
		end := chunkSize + start - 1
		if end > totalSize-1 {
			end = totalSize - 1
		}

		chunks = append(chunks, chunk{start, end, make(chan []byte, 1)})
	}

	return chunks
}

var videoRegexpList = []*regexp.Regexp{
	regexp.MustCompile(`(?:v|embed|shorts|watch\?v)(?:=|/)([^"&?/=%]{11})`),
	regexp.MustCompile(`(?:=|/)([^"&?/=%]{11})`),
	regexp.MustCompile(`([^"&?/=%]{11})`),
}

// ExtractVideoID extracts the videoID from the given string
func ExtractVideoID(videoID string) (string, error) {
	if strings.Contains(videoID, "youtu") || strings.ContainsAny(videoID, "\"?&/<%=") {
		for _, re := range videoRegexpList {
			if isMatch := re.MatchString(videoID); isMatch {
				subs := re.FindStringSubmatch(videoID)
				videoID = subs[1]
			}
		}
	}

	if strings.ContainsAny(videoID, "?&/<%=") {
		return "", ErrInvalidCharactersInVideoID
	}

	if len(videoID) < 10 {
		return "", ErrVideoIDMinLength
	}

	return videoID, nil
}

func FormatBytes(bytes int64) string {
	if bytes < 0 {
		return "Invalid size" // Or handle error as needed
	}
	if bytes == 0 {
		return "0 Bytes"
	}

	const (
		// Use 1024 as the base for binary prefixes (KiB, MiB, GiB, etc.)
		unit = 1024
	)

	// Define the suffixes
	suffixes := []string{"Bytes", "KB", "MB", "GB", "TB", "PB", "EB", "ZB", "YB"}

	// Calculate the appropriate suffix index
	i := int(math.Floor(math.Log(float64(bytes)) / math.Log(unit)))

	// Handle the case where the bytes value is very large and exceeds our suffixes
	if i >= len(suffixes) {
		i = len(suffixes) - 1 // Cap it at the largest suffix we have
	}

	// Calculate the value in the chosen unit
	value := float64(bytes) / math.Pow(unit, float64(i))

	// Format the string, typically with one decimal place unless it's a whole number
	// Use %g to avoid trailing zeros for whole numbers, but ensure one decimal for others
	// Or use %.1f for consistent one decimal place if preferred
	return fmt.Sprintf("%.1f %s", value, suffixes[i])
}