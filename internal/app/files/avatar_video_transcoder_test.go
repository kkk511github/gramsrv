package files

import (
	"context"
	"os/exec"
	"testing"
)

func TestFFmpegAvatarVideoTranscoderProducesTelegramVariants(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	input, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=size=320x240:rate=60", "-t", "0.5", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-movflags", "frag_keyframe+empty_moov", "-f", "mp4", "pipe:1").Output()
	if err != nil {
		t.Fatalf("create test video: %v", err)
	}
	transcoder, err := NewFFmpegAvatarVideoTranscoder()
	if err != nil {
		t.Fatal(err)
	}
	video, err := transcoder.Transcode(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if video.Small.Width != 160 || video.Small.Height != 160 || video.Large.Width != 800 || video.Large.Height != 800 {
		t.Fatalf("unexpected variants: small=%dx%d large=%dx%d", video.Small.Width, video.Small.Height, video.Large.Width, video.Large.Height)
	}
	if len(video.Small.Data) == 0 || len(video.Large.Data) == 0 {
		t.Fatal("empty transcoded avatar variant")
	}
}
