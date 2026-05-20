package handlers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
	"github.com/gofiber/fiber/v2"
)

// MediaHandler uploads images to Cloudinary and returns HTTPS URLs.
type MediaHandler struct {
	cld *cloudinary.Cloudinary
}

// NewMediaHandler returns a handler; if Cloudinary env is missing, Upload returns 503.
func NewMediaHandler() (*MediaHandler, error) {
	cloudName := os.Getenv("CLOUDINARY_CLOUD_NAME")
	key := os.Getenv("CLOUDINARY_API_KEY")
	secret := os.Getenv("CLOUDINARY_API_SECRET")
	if cloudName == "" || key == "" || secret == "" {
		return &MediaHandler{cld: nil}, nil
	}
	cld, err := cloudinary.NewFromParams(cloudName, key, secret)
	if err != nil {
		return nil, err
	}
	return &MediaHandler{cld: cld}, nil
}

// UploadImage is protected upload (teacher/admin).
func (h *MediaHandler) UploadImage(c *fiber.Ctx) error {
	return h.uploadImageCore(c)
}

// UploadImagePublic is a public upload endpoint (no auth middleware).
func (h *MediaHandler) UploadImagePublic(c *fiber.Ctx) error {
	return h.uploadImageCore(c)
}

func (h *MediaHandler) uploadImageCore(c *fiber.Ctx) error {
	if h.cld == nil {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Cloudinary is not configured (set CLOUDINARY_CLOUD_NAME, CLOUDINARY_API_KEY, CLOUDINARY_API_SECRET)",
		})
	}

	fh, err := c.FormFile("file")
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "multipart field 'file' is required",
		})
	}

	f, err := fh.Open()
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "could not read uploaded file"})
	}
	defer f.Close()

	maxMB := 10
	if v := os.Getenv("MAX_UPLOAD_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxMB = n
		}
	}
	maxBytes := int64(maxMB) * 1024 * 1024
	if fh.Size > maxBytes {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("file too large (max %d MB)", maxMB),
		})
	}

	folder := os.Getenv("CLOUDINARY_FOLDER")
	if folder == "" {
		folder = "quizzz/questions"
	}

	ctx := context.Background()
	res, err := h.cld.Upload.Upload(ctx, f, uploader.UploadParams{
		Folder: folder,
	})
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("cloudinary upload failed: %v", err),
		})
	}

	return c.JSON(fiber.Map{
		"url":       res.SecureURL,
		"public_id": res.PublicID,
		"format":    res.Format,
		"width":     res.Width,
		"height":    res.Height,
		"bytes":     res.Bytes,
	})
}
