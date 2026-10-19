package worker

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type voiceStats struct {
	packets, audio               int
	start, end, summed, declared float64
}

func (s *voiceStats) consume(line string) error {
	parts := strings.Split(line, "|")
	values := make(map[string]string, len(parts))
	for _, part := range parts[1:] {
		key, value, ok := strings.Cut(part, "=")
		if ok {
			values[key] = value
		}
	}
	switch parts[0] {
	case "stream":
		if values["codec_type"] != "audio" || values["codec_name"] == "" || values["codec_name"] == "unknown" {
			return fmt.Errorf("voice must contain only supported audio")
		}
		s.audio++
	case "format":
		value, err := voiceNumber(values["duration"])
		if err != nil {
			return err
		}
		s.declared = value
	case "packet":
		s.packets++
		if s.packets > 100000 {
			return fmt.Errorf("voice packet count exceeds limit")
		}
		stamp := values["pts_time"]
		if stamp == "" || stamp == "N/A" {
			stamp = values["dts_time"]
		}
		start, err := voiceNumber(stamp)
		if err != nil {
			return err
		}
		duration, err := voiceNumber(values["duration_time"])
		if err != nil || duration < 0 {
			return fmt.Errorf("invalid packet duration")
		}
		s.start = math.Min(s.start, start)
		s.end = math.Max(s.end, start+duration)
		s.summed += duration
	}
	if math.Max(s.declared, math.Max(s.end-s.start, s.summed)) > 300 {
		return fmt.Errorf("voice exceeds 5 minutes")
	}
	return nil
}

func voiceNumber(value string) (float64, error) {
	if value == "" || value == "N/A" {
		return 0, nil
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, fmt.Errorf("invalid ffprobe duration or timestamp")
	}
	return n, nil
}

func (s voiceStats) duration() (float64, error) {
	seconds := math.Max(s.declared, math.Max(s.end-s.start, s.summed))
	if s.audio != 1 || s.packets == 0 || seconds <= 0 || seconds > 300 {
		return 0, fmt.Errorf("invalid voice streams or duration")
	}
	return seconds, nil
}
