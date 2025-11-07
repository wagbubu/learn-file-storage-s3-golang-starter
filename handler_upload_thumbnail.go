package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	// TODO: implement the upload here
	const maxMemory = 10 << 20
	r.ParseMultipartForm(maxMemory)

	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	mediaType := header.Header.Get("Content-Type")
	mimeType, _, err := mime.ParseMediaType(mediaType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "error parsing media type", err)
		return
	}

	if mimeType != "image/jpeg" && mimeType != "image/png" {
		respondWithError(w, http.StatusBadRequest, "file must be an image jpeg/png", nil)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			respondWithError(w, http.StatusNotFound, "not found", err)
		default:
			respondWithError(w, http.StatusInternalServerError, "something went wrong!", err)
		}
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "unauthorized", nil)
		return
	}

	bytes := make([]byte, 32)

	rand.Read(bytes)
	fileName := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(bytes)

	fileNameAndExtension := fmt.Sprintf("%s.%s", fileName, strings.Split(mediaType, "/")[1])
	filePath := filepath.Join(cfg.assetsRoot, fileNameAndExtension)

	localFile, err := os.Create(filePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error creating file", err)
		return
	}

	io.Copy(localFile, file)

	thumbnailURL := fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, fileNameAndExtension)
	video.ThumbnailURL = &thumbnailURL
	cfg.db.UpdateVideo(video)

	respondWithJSON(w, http.StatusOK, database.Video{ID: videoID, CreatedAt: video.CreatedAt, UpdatedAt: video.UpdatedAt, ThumbnailURL: video.ThumbnailURL, VideoURL: video.VideoURL, CreateVideoParams: video.CreateVideoParams})
}
