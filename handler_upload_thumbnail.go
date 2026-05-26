package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
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
	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, 500, "issues", err)
		return
	}

	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse from file", err)
		return
	}

	defer file.Close()

	mT, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil || (mT != "image/png" && mT != "image/jpeg") {
		respondWithError(w, http.StatusUnsupportedMediaType, "wrong thumbnail format", err)
		return
	}
	/*
		fileData, err := io.ReadAll(file)
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "Unable to read file", err)
			return
		}
	*/
	// get video metadata
	v, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Video not found", err)
		return
	}
	if v.ID == uuid.Nil {
		respondWithError(w, http.StatusNotFound, "Video does not exist", err)
		return
	}
	if v.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", err)
		return
	}
	randomBytes := make([]byte, 32)
	rand.Read(randomBytes)
	randomString := base64.RawURLEncoding.EncodeToString(randomBytes)
	fileExtension := strings.Split(mT, "/")[1]
	fP := filepath.Join(cfg.assetsRoot, randomString) + "." + fileExtension
	newFile, err := os.Create(fP)
	if err != nil {
		respondWithError(w, 500, "cannot create new file", err)
		return
	}
	_, err = io.Copy(newFile, file)
	if err != nil {
		respondWithError(w, 500, "something went wrong", err)
		return
	}
	tURL := fmt.Sprintf("http://localhost:%s/assets/%s.%s", cfg.port, randomString, fileExtension)

	v.ThumbnailURL = &tURL

	err = cfg.db.UpdateVideo(v)
	if err != nil {
		respondWithError(w, 500, "unable to update video data", err)
		return
	}
	respondWithJSON(w, http.StatusOK, v)
}
