package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	var maxBytesErr *http.MaxBytesError
	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)

	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "unable to parse video id", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "unauthorized", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "unauthorized", err)
		return
	}

	videoFile, header, err := r.FormFile("video")
	if err != nil {
		switch {
		case errors.As(err, &maxBytesErr):
			respondWithError(w, http.StatusRequestEntityTooLarge, "file size exceeded", maxBytesErr)
		default:
			respondWithError(w, http.StatusBadRequest, "cannot read video", err)
		}
		return
	}
	defer videoFile.Close()

	mediaType := header.Header.Get("Content-Type")
	mimeType, _, err := mime.ParseMediaType(mediaType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "cannot parse content type", err)
		return
	}

	if mimeType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "file is not a video of type .mp4", nil)
		return
	}
	ext := strings.Split(mimeType, "/")[1]

	tmpFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		log.Fatal("failed to create temp file")
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	_, err = io.Copy(tmpFile, videoFile)
	if err != nil {
		log.Fatal("failed to copy temp file")
	}

	_, err = tmpFile.Seek(0, io.SeekStart)
	if err != nil {
		log.Fatal("failed to reset temp file pointer")
	}

	processedPath, err := processVideoForFastStart(tmpFile.Name())
	if err != nil {
		log.Fatal(err)
	}

	pf, err := os.Open(processedPath)
	if err != nil {
		log.Fatal("error failed to open procesed path")
	}
	defer os.Remove(processedPath)
	defer pf.Close()

	videoOrientation, err := getVideoAspectRatio(pf.Name())
	if err != nil {
		log.Fatal("error getting video aspect")
	}

	var s3folderName string
	switch videoOrientation {
	case "16:9":
		s3folderName = "landscape"
	case "9:16":
		s3folderName = "portrait"
	default:
		s3folderName = "other"
	}

	bytes := make([]byte, 32)
	rand.Read(bytes)
	fname := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(bytes)

	fullFname := fmt.Sprintf("%s/%s.%s", s3folderName, fname, ext)
	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(fullFname),
		Body:        pf,
		ContentType: aws.String(mimeType),
	})

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "s3 upload failed", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "video not found", err)
		return
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusForbidden, "forbidden", nil)
		return
	}

	s3VideoURL := fmt.Sprintf("https://%s/%s", cfg.s3CfDistribution, fullFname)

	video.UpdatedAt = time.Now().UTC()
	video.VideoURL = &s3VideoURL

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to save to db", err)
		return
	}

	// presignedVideo, err := cfg.dbVideoToSignedVideo(video)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	respondWithJSON(w, http.StatusOK, video)
}
