package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os/exec"
	"path/filepath"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/ffprobe"
)

const TOLERANCE = 0.1

var (
	VERTICAL_RATIO   = math.Round((9.0/16.0)*100) / 100
	HORIZONTAL_RATIO = math.Round((16.0/9.0)*100) / 100
)

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)

	var probeRes ffprobe.Response

	out := &bytes.Buffer{}
	cmd.Stdout = out

	err := cmd.Run()
	if err != nil {
		return "", err
	}

	err = json.Unmarshal(out.Bytes(), &probeRes)
	if err != nil {
		return "", err
	}

	var vd struct {
		height float64
		width  float64
	}
	var found bool

	for _, s := range probeRes.Streams {
		if s.CodecType == "video" {
			found = true
			vd.height = float64(s.Height)
			vd.width = float64(s.Width)
			break
		}
	}
	if !found {
		return "", errors.New("no video stream found")
	}

	ratio := getRatio(vd.width, vd.height, 2)
	if withinTolerance(HORIZONTAL_RATIO, ratio, TOLERANCE) {
		return "16:9", nil
	} else if withinTolerance(VERTICAL_RATIO, ratio, TOLERANCE) {
		return "9:16", nil
	} else {
		return "other", nil
	}
}

func getRatio(numerator, denominator, decimal float64) float64 {
	multiplier := math.Pow(10, decimal)
	return math.Round((numerator/denominator)*multiplier) / multiplier
}

func withinTolerance(base, value, tolerance float64) bool {
	min := base - tolerance
	max := base + tolerance
	if value >= min && value <= max {
		return true
	}
	return false
}

func processVideoForFastStart(filePath string) (string, error) {
	dir := filepath.Dir(filePath)
	base := filepath.Base(filePath)
	out := filepath.Join(dir, base+".processing")

	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", out)

	err := cmd.Run()
	if err != nil {
		return "", err
	}

	return out, nil
}

// func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
// 	pc := s3.NewPresignClient(s3Client)
// 	input := s3.GetObjectInput{
// 		Bucket: &bucket,
// 		Key:    &key,
// 	}
// 	preq, err := pc.PresignGetObject(context.Background(), &input, s3.WithPresignExpires(expireTime))
// 	if err != nil {
// 		return "", err
// 	}

// 	return preq.URL, nil
// }

// func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
// 	videoURL := video.VideoURL
// 	if videoURL == nil {
// 		return video, nil
// 	}

// 	bk := strings.Split(*videoURL, ",")
// 	if len(bk) != 2 {
// 		return database.Video{}, errors.New("missing key or bucket")
// 	}
// 	bucket := strings.TrimSpace(bk[0])
// 	key := strings.TrimSpace(bk[1])

// 	if bucket == "" || key == "" {
// 		return database.Video{}, errors.New("missing key or bucket")
// 	}

// 	url, err := generatePresignedURL(cfg.s3Client, bucket, key, 2*time.Minute)
// 	if err != nil {
// 		return database.Video{}, err
// 	}

// 	v := video
// 	v.VideoURL = &url

// 	return v, nil
// }
