package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/cheggaaa/pb/v3"
)

type Track struct {
	StartTime      float64
	EndTime        float64
	MainArtist     string
	MainTitle      string
	MainLabel      string
	Additional     []AdditionalTrack
	OutputFilename string
}

type AdditionalTrack struct {
	Artist string
	Title  string
	Label  string
}

var (
	tracklistPath = flag.String("tracklist", "", "Path to tracklist file")
	audioFlag     = flag.Bool("audio", false, "Output audio (mp3)")
	videoFlag     = flag.Bool("video", false, "Output video (mp4)")
	inputPath     = flag.String("input", "", "Input media file")
)

const (
	maxWorkers    = 4
	outputDir     = "output"
	timeFormat    = "15:04:05"
	metadataAlbum = "Ultra Europe 2025"
)



func main() {
	flag.Parse()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	if err := validateFlags(); err != nil {
		logger.Error("Validation error", "error", err)
		os.Exit(1)
	}

	tracks, album, err := parseTracklist(*tracklistPath)
	if err != nil {
		logger.Error("Failed to parse tracklist", "error", err)
		os.Exit(1)
	}

  logger.Info("Parsed tracklist", "album", album, "trackCount", len(tracks))

	duration, err := getMediaDuration(*inputPath)
	if err != nil {
		logger.Error("Failed to get media duration", "error", err)
		os.Exit(1)
	}

	if err := prepareOutputDir(); err != nil {
		logger.Error("Output directory preparation failed", "error", err)
		os.Exit(1)
	}

	calculateEndTimes(tracks, duration)

	outputExt := getOutputExtension()
	createFilenames(tracks, outputExt)

	processTracksConcurrently(tracks, *inputPath, outputExt, album, logger)
}

func validateFlags() error {
	if *tracklistPath == "" || *inputPath == "" {
		return errors.New("both --tracklist and --input are required")
	}
	if !*audioFlag && !*videoFlag {
		return errors.New("either --audio or --video must be specified")
	}
	if *audioFlag && *videoFlag {
		return errors.New("cannot specify both --audio and --video")
	}
	return nil
}

func parseTracklist(path string) ([]Track, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Scan()
	header := scanner.Text()

	var tracks []Track
	currentTrack := (*Track)(nil)
	lineRe := regexp.MustCompile(`^\[(\d+:?\d*:\d+)\]\s(.+?)(?:\s\[(.+)\])?$`)
	wRe := regexp.MustCompile(`^w/\s(.+?)(?:\s\[(.+)\])?$`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if matches := lineRe.FindStringSubmatch(line); matches != nil {
			if currentTrack != nil {
				tracks = append(tracks, *currentTrack)
			}

			start, err := parseTimestamp(matches[1])
			if err != nil {
				return nil, "", err
			}

			// Skip stage announcement lines
			if strings.HasSuffix(matches[2], "On Stage") {
				continue
			}

			artist, title, err := parseArtistTitle(matches[2])
			if err != nil {
				return nil, "", err
			}

			label := ""
			if len(matches) > 3 && matches[3] != "" {
				label = matches[3]
			}

			currentTrack = &Track{
				StartTime:  start,
				MainArtist: artist,
				MainTitle:  title,
				MainLabel:  label,
			}
		} else if strings.HasPrefix(line, "w/") {
			if currentTrack == nil {
				continue
			}

			matches := wRe.FindStringSubmatch(line)
			if matches == nil {
				continue
			}

			artist, title, err := parseArtistTitle(matches[1])
			if err != nil {
				return nil, "", err
			}

			currentTrack.Additional = append(currentTrack.Additional, AdditionalTrack{
				Artist: artist,
				Title:  title,
				Label:  matches[2],
			})
		}
	}

	if currentTrack != nil {
		tracks = append(tracks, *currentTrack)
	}

  

	return tracks, header, scanner.Err()
}

func parseTimestamp(ts string) (float64, error) {
	parts := strings.Split(ts, ":")
	var total float64

	// Handle different time formats (MM:SS or HH:MM:SS)
	multipliers := []float64{1, 60, 3600}
	for i := range parts {
		val, err := strconv.Atoi(parts[len(parts)-1-i])
		if err != nil {
			return 0, err
		}
		total += float64(val) * multipliers[i]
	}
	return total, nil
}

func parseArtistTitle(s string) (string, string, error) {
	parts := strings.SplitN(s, " - ", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid artist/title format: %s", s)
	}
	return parts[0], parts[1], nil
}

func getMediaDuration(path string) (float64, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", 
		"format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe error: %v", err)
	}

	return strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
}

func prepareOutputDir() error {
	if _, err := os.Stat(outputDir); err == nil {
		fmt.Print("Output directory exists. Delete it? (y/n): ")
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "y" {
			return errors.New("user cancelled operation")
		}
		if err := os.RemoveAll(outputDir); err != nil {
			return err
		}
	}
	return os.Mkdir(outputDir, 0755)
}

func calculateEndTimes(tracks []Track, duration float64) {
	for i := range tracks {
		if i < len(tracks)-1 {
			tracks[i].EndTime = tracks[i+1].StartTime
		} else {
			tracks[i].EndTime = duration
		}
	}
}

func getOutputExtension() string {
	if *audioFlag {
		return ".mp3"
	}
	return ".mp4"
}

func createFilenames(tracks []Track, ext string) {
	for i := range tracks {
		tracks[i].OutputFilename = fmt.Sprintf("output/%02d - %s - %s%s",
			i+1, sanitizeFilename(tracks[i].MainArtist), sanitizeFilename(tracks[i].MainTitle), ext)
	}
}

func sanitizeFilename(name string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) {
			return -1
		}
		return r
	}, name)
}

func processTracksConcurrently(tracks []Track, inputPath, ext, album string, logger *slog.Logger) {
	bar := pb.StartNew(len(tracks))
	defer bar.Finish()

	var wg sync.WaitGroup
	sem := make(chan struct{}, maxWorkers)
	var errCount atomic.Int32


	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up cleanup on interrupt
	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-interruptChan
		logger.Info("Received interrupt signal, cleaning up...")
		cancel()
	}()

	for i := range tracks {
		wg.Add(1)
		go func(t *Track) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
				if err := processTrack(ctx, t, inputPath, album); err != nil {
					logger.Error("Track processing failed", 
						"track", t.MainTitle, "error", err)
					errCount.Add(1)
				}
				bar.Increment()
			case <-ctx.Done():
				return
			}
		}(&tracks[i])
	}

	wg.Wait()

	if errCount.Load() > 0 {
		logger.Error("Completed with errors", "errorCount", errCount.Load())
	}
}

func processTrack(ctx context.Context, t *Track, inputPath, album string) error {
	// Validate time values
	if t.StartTime >= t.EndTime {
		return fmt.Errorf("invalid time range: start(%f) >= end(%f)", t.StartTime, t.EndTime)
	}

	args := []string{
		"-v", "warning",              // Show warnings for debugging
		"-ss", fmt.Sprintf("%f", t.StartTime),
		"-i", inputPath,
		"-t", fmt.Sprintf("%f", t.EndTime-t.StartTime),
		
		// Memory management and optimization
		"-max_muxing_queue_size", "1024",
		"-threads", "2",              // Limit threads per process
	}

	if *videoFlag {
		args = append(args,
			"-c:v", "libx264",           // Use H.264 codec
			"-preset", "veryfast",       // Use faster preset to reduce memory usage
			"-crf", "23",               // Reasonable quality
			"-vsync", "cfr",            // Force constant frame rate
			"-profile:v", "baseline",   // Use baseline profile for better compatibility and less memory
			"-level", "3.0",            // Lower level for less memory usage
			"-tune", "fastdecode",      // Optimize for decoding speed
			"-c:a", "aac",              // AAC audio codec
			"-b:a", "192k",             // Audio bitrate
			"-ac", "2",                 // Force stereo
			"-ar", "48000",             // Standard sample rate
			"-movflags", "+faststart",  // Enable fast start
			"-y",                        // Overwrite output
		)
	} else {
		args = append(args, "-c:a", "libmp3lame", "-q:a", "2")
	}

	metadata := buildMetadata(t, album)
	args = append(args, metadata...)
	args = append(args, t.OutputFilename)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg error: %v\n%s", err, string(output))
	}
	return nil
}

func buildMetadata(t *Track, album string) []string {
	metadata := []string{
		"-metadata", fmt.Sprintf("title=%s", buildTitle(t)),
		"-metadata", fmt.Sprintf("artist=%s", t.MainArtist),
		"-metadata", fmt.Sprintf("album=%s", album),
		"-metadata", fmt.Sprintf("date=%s", "2025"),
		"-metadata", fmt.Sprintf("comment=%s", buildComment(t)),
	}

	if t.MainLabel != "" {
		metadata = append(metadata, "-metadata", fmt.Sprintf("publisher=%s", t.MainLabel))
	}

	return metadata
}

func buildTitle(t *Track) string {
	title := t.MainTitle
	for _, add := range t.Additional {
		title += " / " + add.Title
	}
	return title
}

func buildComment(t *Track) string {
	var comments []string
	for _, add := range t.Additional {
		comments = append(comments, fmt.Sprintf("%s - %s [%s]", 
			add.Artist, add.Title, add.Label))
	}
	return "Additional tracks: " + strings.Join(comments, "; ")
}
