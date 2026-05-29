package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"

	//	"strings"
	//	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"

	//	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	// set upload size limit and apply it to the requests body
	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)

	// get videoID from filepath
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid id", err)
		return
	}

	// authenticate user
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
	fmt.Println("uploading video", videoID, "by user", userID)

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
	// actual video upload
	// unlike with the thumbnail upload, ParseMultipartForm is not used here, since we don´t want whole videos stored in RAM

	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse from file", err)
		return
	}

	defer file.Close()

	mT, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil || mT != "video/mp4" {
		respondWithError(w, http.StatusUnsupportedMediaType, "wrong video format", err)
		return
	}
	f, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, 500, "issues creating temp file", err)
		return
	}

	defer os.Remove(f.Name())

	defer f.Close()

	// copy from the multipartfile to the temporary file
	_, err = io.Copy(f, file)
	if err != nil {
		respondWithError(w, 500, "something went wrong", err)
		return
	}

	// reset the pointer of the temporary file to be able to read the file from the beginning again
	_, err = f.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, 500, "reset unsuccessful", err)
		return
	}
	// process the videofile for fast start before uploading
	processedFilePath, err := processVideoForFastStart(f.Name())
	if err != nil {
		respondWithError(w, 500, "video processing failed", err)
		return
	}
	pF, err := os.Open(processedFilePath)
	if err != nil {
		respondWithError(w, 500, "unable to open file", err)
		return
	}
	// creaate random key for file identification using 32 byte exist

	randomBytes := make([]byte, 32)
	rand.Read(randomBytes)
	randomString := base64.RawURLEncoding.EncodeToString(randomBytes)
	format, err := getVideoAspectRatio(f.Name())
	if err != nil {
		respondWithError(w, 500, err.Error(), err)
		return
	}
	fK := format + "/" + randomString + ".mp4"

	// create params struct for PutObject method of s3 client.
	params := &s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(fK),
		Body:        pF,
		ContentType: aws.String(mT),
	}

	// use client.PutObject to place the file in s3Bucket
	_, err = cfg.s3Client.PutObject(context.Background(), params)
	if err != nil {
		respondWithError(w, 500, "PutObject failed", err)
		return
	}

	// update the video Url in the database
	vURL := fmt.Sprintf("https://%s/%s", cfg.s3CfDistribution, fK)

	v.VideoURL = &vURL

	err = cfg.db.UpdateVideo(v)
	if err != nil {
		respondWithError(w, 500, "unable to update video", err)
		return
	}

	respondWithJSON(w, http.StatusCreated, v)
}
