package worker

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

type VoiceProbe interface {
	Duration(context.Context, []byte, string) (float64, error)
}

type FFProbe struct{}

func (FFProbe) Duration(ctx context.Context, data []byte, mime string) (float64, error) {
	demuxer := ""
	switch mime {
	case "audio/webm":
		demuxer = "matroska"
	case "audio/ogg":
		demuxer = "ogg"
	case "audio/mpeg":
		demuxer = "mp3"
	case "audio/wave":
		demuxer = "wav"
	default:
		return 0, fmt.Errorf("unsupported voice type")
	}
	// A seekable private file also works for Ogg/WebM recordings without duration tags.
	file, err := os.CreateTemp("", "pulsechat-voice-*")
	if err != nil {
		return 0, err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return 0, writeErr
	}
	if closeErr != nil {
		return 0, closeErr
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-protocol_whitelist", "file,pipe",
		"-f", demuxer, "-show_entries", "packet=pts_time,dts_time,duration_time:stream=codec_type,codec_name:format=duration",
		"-of", "compact=p=1:nk=0", "-i", file.Name())
	cmd.WaitDelay = time.Second
	cmd.Stderr = io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, err
	}
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start ffprobe: %w", err)
	}
	stats := voiceStats{}
	lines := bufio.NewScanner(stdout)
	lines.Buffer(make([]byte, 4096), 8192)
	for lines.Scan() {
		if err := stats.consume(lines.Text()); err != nil {
			cancel()
			_ = cmd.Wait()
			return 0, err
		}
	}
	scanErr := lines.Err()
	if scanErr != nil {
		cancel()
	}
	if err := cmd.Wait(); err != nil {
		return 0, fmt.Errorf("ffprobe failed: %w", err)
	}
	if scanErr != nil {
		return 0, fmt.Errorf("read ffprobe output: %w", scanErr)
	}
	return stats.duration()
}
