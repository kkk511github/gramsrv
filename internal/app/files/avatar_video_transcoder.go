package files

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	avatarVideoTranscodeTimeout       = 45 * time.Second
	avatarVideoTranscodeMaxInput      = 64 << 20
	avatarVideoTranscodeMaxOutput     = 32 << 20
	avatarVideoTranscodeMaxConcurrent = 2
)

type AvatarVideoVariant struct {
	Data     []byte
	Width    int
	Height   int
	Duration float64
}

type AvatarVideo struct {
	Small AvatarVideoVariant
	Large AvatarVideoVariant
}

// AvatarVideoTranscoder produces Telegram's p (small preview) and u (full avatar) MP4 variants.
type AvatarVideoTranscoder interface {
	Transcode(ctx context.Context, data []byte) (AvatarVideo, error)
}

type FFmpegAvatarVideoTranscoder struct {
	ffmpeg  string
	ffprobe string
	timeout time.Duration
	slots   chan struct{}
}

func NewFFmpegAvatarVideoTranscoder() (*FFmpegAvatarVideoTranscoder, error) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, err
	}
	ffprobeName := "ffprobe"
	if ext := filepath.Ext(ffmpeg); ext != "" {
		ffprobeName += ext
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), ffprobeName)
	if _, err := os.Stat(ffprobe); err != nil {
		ffprobe, err = exec.LookPath("ffprobe")
		if err != nil {
			return nil, err
		}
	}
	return &FFmpegAvatarVideoTranscoder{
		ffmpeg: ffmpeg, ffprobe: ffprobe, timeout: avatarVideoTranscodeTimeout,
		slots: make(chan struct{}, avatarVideoTranscodeMaxConcurrent),
	}, nil
}

func (t *FFmpegAvatarVideoTranscoder) Transcode(ctx context.Context, data []byte) (AvatarVideo, error) {
	if t == nil || t.ffmpeg == "" || t.ffprobe == "" {
		return AvatarVideo{}, fmt.Errorf("avatar video transcoder unavailable")
	}
	if len(data) == 0 || len(data) > avatarVideoTranscodeMaxInput {
		return AvatarVideo{}, fmt.Errorf("avatar video input size out of range: %d", len(data))
	}
	select {
	case t.slots <- struct{}{}:
		defer func() { <-t.slots }()
	case <-ctx.Done():
		return AvatarVideo{}, ctx.Err()
	}

	runCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()
	input, err := os.CreateTemp("", "safelink-avatar-input-*.mp4")
	if err != nil {
		return AvatarVideo{}, fmt.Errorf("create avatar video input: %w", err)
	}
	inputPath := input.Name()
	defer os.Remove(inputPath)
	if _, err := input.Write(data); err != nil {
		input.Close()
		return AvatarVideo{}, fmt.Errorf("write avatar video input: %w", err)
	}
	if err := input.Close(); err != nil {
		return AvatarVideo{}, fmt.Errorf("close avatar video input: %w", err)
	}

	small, err := t.transcodeVariant(runCtx, inputPath, 160, "p")
	if err != nil {
		return AvatarVideo{}, err
	}
	large, err := t.transcodeVariant(runCtx, inputPath, 800, "u")
	if err != nil {
		return AvatarVideo{}, err
	}
	return AvatarVideo{Small: small, Large: large}, nil
}

func (t *FFmpegAvatarVideoTranscoder) transcodeVariant(ctx context.Context, inputPath string, size int, label string) (AvatarVideoVariant, error) {
	output, err := os.CreateTemp("", "safelink-avatar-"+label+"-*.mp4")
	if err != nil {
		return AvatarVideoVariant{}, fmt.Errorf("create avatar video output: %w", err)
	}
	outputPath := output.Name()
	output.Close()
	defer os.Remove(outputPath)

	filter := fmt.Sprintf("crop=min(iw\\,ih):min(iw\\,ih),scale=%d:%d:flags=lanczos,fps=30", size, size)
	cmd := exec.CommandContext(ctx, t.ffmpeg,
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", inputPath, "-map", "0:v:0", "-an", "-sn", "-dn", "-map_metadata", "-1",
		"-vf", filter,
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "23",
		"-profile:v", "main", "-level", "3.1", "-pix_fmt", "yuv420p",
		"-movflags", "+faststart", outputPath)
	stderr, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return AvatarVideoVariant{}, ctx.Err()
	}
	if err != nil {
		return AvatarVideoVariant{}, commandError("ffmpeg avatar video transcode", err, stderr)
	}
	info, err := os.Stat(outputPath)
	if err != nil || info.Size() <= 0 || info.Size() > avatarVideoTranscodeMaxOutput {
		return AvatarVideoVariant{}, fmt.Errorf("avatar video output size invalid")
	}
	variant, err := t.probe(ctx, outputPath)
	if err != nil {
		return AvatarVideoVariant{}, err
	}
	if variant.Width != size || variant.Height != size {
		return AvatarVideoVariant{}, fmt.Errorf("avatar video dimensions = %dx%d, want %dx%d", variant.Width, variant.Height, size, size)
	}
	variant.Data, err = os.ReadFile(outputPath)
	if err != nil {
		return AvatarVideoVariant{}, fmt.Errorf("read avatar video output: %w", err)
	}
	return variant, nil
}

func (t *FFmpegAvatarVideoTranscoder) probe(ctx context.Context, path string) (AvatarVideoVariant, error) {
	cmd := exec.CommandContext(ctx, t.ffprobe, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=codec_name,pix_fmt,width,height,duration:format=duration", "-of", "json", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return AvatarVideoVariant{}, commandError("ffprobe avatar video output", err, out)
	}
	var result struct {
		Streams []struct {
			CodecName string `json:"codec_name"`
			PixelFmt  string `json:"pix_fmt"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
			Duration  string `json:"duration"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &result); err != nil || len(result.Streams) != 1 {
		return AvatarVideoVariant{}, fmt.Errorf("invalid ffprobe avatar video metadata")
	}
	stream := result.Streams[0]
	duration := parsePositiveFloat(stream.Duration)
	if duration == 0 {
		duration = parsePositiveFloat(result.Format.Duration)
	}
	if stream.CodecName != "h264" || stream.PixelFmt != "yuv420p" || stream.Width <= 0 || stream.Height <= 0 || duration <= 0 {
		return AvatarVideoVariant{}, fmt.Errorf("unsupported avatar video output")
	}
	return AvatarVideoVariant{Width: stream.Width, Height: stream.Height, Duration: duration}, nil
}
