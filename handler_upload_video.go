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
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
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

	bytes := make([]byte, 32)
	rand.Read(bytes)
	fname := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(bytes)

	fullFname := fmt.Sprintf("%s.%s", fname, ext)
	cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(fullFname),
		Body:        tmpFile,
		ContentType: aws.String(mimeType),
	})

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "video not found", err)
		return
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusForbidden, "forbidden", nil)
		return
	}

	s3VideoURL := fmt.Sprintf("https://%s.s3.ap-southeast-2.amazonaws.com/%s", cfg.s3Bucket, fullFname)

	updatedVideo := database.Video{
		ID:                video.ID,
		CreatedAt:         video.CreatedAt,
		UpdatedAt:         time.Now().UTC(),
		ThumbnailURL:      video.ThumbnailURL,
		VideoURL:          &s3VideoURL,
		CreateVideoParams: video.CreateVideoParams,
	}

	err = cfg.db.UpdateVideo(updatedVideo)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "failed to save to db", err)
		return
	}
}
