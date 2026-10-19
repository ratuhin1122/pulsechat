package worker

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"pulsechat/internal/upload"
	"testing"
)

func sampleWAV(seconds int) []byte {
	const rate = 8000
	data := make([]byte, 44+rate*seconds)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], rate)
	binary.LittleEndian.PutUint32(data[28:], rate)
	binary.LittleEndian.PutUint16(data[32:], 1)
	binary.LittleEndian.PutUint16(data[34:], 8)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(rate*seconds))
	for i := 44; i < len(data); i++ {
		data[i] = 128
	}
	return data
}

func TestFFProbeWAV(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	for _, tt := range []struct {
		name string
		data []byte
		bad  bool
		want float64
	}{
		{name: "one second", data: sampleWAV(1), want: 1},
		{name: "five minutes", data: sampleWAV(300), want: 300},
		{name: "over five minutes", data: sampleWAV(301), bad: true},
		{name: "invalid", data: []byte("not audio"), bad: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (FFProbe{}).Duration(context.Background(), tt.data, "audio/wave")
			if (err != nil) != tt.bad || err == nil && math.Abs(got-tt.want) > 0.001 {
				t.Fatalf("duration=%v,error=%v", got, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (FFProbe{}).Duration(ctx, sampleWAV(1), "audio/wave"); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestFFProbeFormats(t *testing.T) {
	for _, name := range []string{"ffprobe", "ffmpeg"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Skip(name + " not installed")
		}
	}
	directory := t.TempDir()
	source := filepath.Join(directory, "source.wav")
	if err := os.WriteFile(source, sampleWAV(1), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct{ ext, mime, codec string }{
		{"mp3", "audio/mpeg", "libmp3lame"}, {"ogg", "audio/ogg", "libopus"}, {"webm", "audio/webm", "libopus"},
	} {
		t.Run(tt.ext, func(t *testing.T) {
			output := filepath.Join(directory, "voice."+tt.ext)
			cmd := exec.CommandContext(context.Background(), "ffmpeg", "-v", "error", "-i", source, "-c:a", tt.codec, "-y", output)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("create sample: %v: %s", err, out)
			}
			data, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			if err := upload.Recheck("voice", "voice."+tt.ext, tt.mime, data); err != nil {
				t.Fatalf("type re-check: %v", err)
			}
			got, err := (FFProbe{}).Duration(context.Background(), data, tt.mime)
			if err != nil || got < 0.9 || got > 1.2 {
				t.Fatalf("duration=%v,error=%v", got, err)
			}
		})
	}
}
