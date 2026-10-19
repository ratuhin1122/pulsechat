package worker

import (
	"strings"
	"testing"
)

func TestVoiceOutput(t *testing.T) {
	for _, tt := range []struct {
		name, output string
		want         float64
		bad          bool
	}{
		{name: "WAV", output: "packet|pts_time=0|duration_time=1\nstream|codec_name=pcm_s16le|codec_type=audio\nformat|duration=1", want: 1},
		{name: "no duration tag", output: "packet|pts_time=0|duration_time=0.02\npacket|pts_time=1|duration_time=0.02\nstream|codec_type=audio|codec_name=opus", want: 1.02},
		{name: "five minutes", output: "packet|pts_time=0|duration_time=300\nstream|codec_type=audio|codec_name=opus", want: 300},
		{name: "over five minutes", output: "packet|pts_time=0|duration_time=300.01", bad: true},
		{name: "underreported tag", output: "packet|pts_time=301|duration_time=1\nformat|duration=1", bad: true},
		{name: "overreported tag", output: "format|duration=301", bad: true},
		{name: "NaN", output: "format|duration=NaN", bad: true},
		{name: "infinity", output: "packet|pts_time=Inf", bad: true},
		{name: "video stream", output: "stream|codec_type=video|codec_name=vp9", bad: true},
		{name: "unknown codec", output: "stream|codec_type=audio|codec_name=unknown", bad: true},
		{name: "no packets", output: "stream|codec_type=audio|codec_name=opus\nformat|duration=1", bad: true},
		{name: "two audio streams", output: "packet|pts_time=0|duration_time=1\nstream|codec_type=audio|codec_name=opus\nstream|codec_type=audio|codec_name=opus", bad: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stats := voiceStats{}
			var err error
			for _, line := range strings.Split(tt.output, "\n") {
				if err = stats.consume(line); err != nil {
					break
				}
			}
			var got float64
			if err == nil {
				got, err = stats.duration()
			}
			if (err != nil) != tt.bad || err == nil && got != tt.want {
				t.Fatalf("duration=%v, error=%v", got, err)
			}
		})
	}
}
