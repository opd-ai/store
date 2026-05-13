package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/opd-ai/store/pkg/handler"
	"github.com/opd-ai/store/pkg/models"
)

// DigitalMediaHandler implements FulfillmentHandler for instant digital downloads.
// It supports both local filesystem and S3 storage backends.
type DigitalMediaHandler struct{}

// NewDigitalMediaHandler creates a new DigitalMediaHandler instance.
func NewDigitalMediaHandler() *DigitalMediaHandler {
	return &DigitalMediaHandler{}
}

// Handle executes the digital media fulfillment process.
// It generates a download URL (pre-signed S3 URL or direct link) and returns it to the client.
func (h *DigitalMediaHandler) Handle(ctx context.Context, payment *models.Payment, item *models.Item) (map[string]interface{}, error) {
	// Verify payment is confirmed
	if payment.Status != "confirmed" {
		return nil, fmt.Errorf("payment not confirmed")
	}

	config := handler.Config{Settings: item.BackendConfig}

	// Get storage type
	storage := config.GetString("storage")
	if storage == "" {
		storage = "local"
	}

	var downloadURL string
	var fileSize int64

	if storage == "s3" {
		// For S3, generate a presigned URL
		url, err := h.generateS3URL(ctx, item, config)
		if err != nil {
			return nil, err
		}
		downloadURL = url
		// In a real implementation, would fetch actual file size from S3
		fileSize = 0
	} else {
		// For local storage, return a direct download link
		filePath := config.GetString("file_path")
		if filePath == "" {
			return nil, fmt.Errorf("file_path not configured")
		}
		downloadURL = fmt.Sprintf("/api/download/%s", item.ID)
		// Would get actual file size from filesystem in real impl
		fileSize = 0
	}

	// Calculate expiration time
	expirationHours := config.GetInt("expiration_hours")
	if expirationHours == 0 {
		expirationHours = 24 // Default to 24 hours
	}
	expiresAt := time.Now().Add(time.Duration(expirationHours) * time.Hour)

	result := map[string]interface{}{
		"download_url":  downloadURL,
		"expires_at":    expiresAt.Format(time.RFC3339),
		"file_size_mb":  fileSize / (1024 * 1024),
		"max_downloads": config.GetInt("max_downloads"),
	}

	return result, nil
}

// Validate checks if the digital media configuration is valid.
func (h *DigitalMediaHandler) Validate(config models.JSONMap) error {
	c := handler.Config{Settings: config}

	storage := c.GetString("storage")
	if storage == "" {
		storage = "local"
	}

	switch storage {
	case "s3":
		// Validate S3 configuration
		bucket := c.GetString("s3_bucket")
		if bucket == "" {
			return fmt.Errorf("s3_bucket is required for S3 storage")
		}
		region := c.GetString("s3_region")
		if region == "" {
			return fmt.Errorf("s3_region is required for S3 storage")
		}
	case "local":
		// Validate local storage
		filePath := c.GetString("file_path")
		if filePath == "" {
			return fmt.Errorf("file_path is required for local storage")
		}
	default:
		return fmt.Errorf("unsupported storage type: %s (must be 's3' or 'local')", storage)
	}

	// Validate expiration if specified
	expiration := c.GetInt("expiration_hours")
	if expiration < 1 {
		return fmt.Errorf("expiration_hours must be at least 1")
	}

	return nil
}

// Metadata returns handler metadata for discovery and admin UI.
func (h *DigitalMediaHandler) Metadata() handler.HandlerMetadata {
	return handler.HandlerMetadata{
		Type:        "digital_media",
		DisplayName: "Digital Media Download",
		Description: "Deliver digital products (ebooks, software, assets) via instant download. Supports both S3 and local filesystem storage with expiring download links.",
		RequiredFields: []handler.Field{
			{
				Name:        "storage",
				Type:        "string",
				Description: "Storage backend type (s3 or local)",
				Example:     "s3",
				Validation:  "^(s3|local)$",
				Required:    false,
			},
		},
		OptionalFields: []handler.Field{
			{
				Name:        "file_path",
				Type:        "string",
				Description: "Local file path (required if storage=local)",
				Example:     "./downloads/product.pdf",
				Validation:  "",
				Required:    false,
			},
			{
				Name:        "s3_bucket",
				Type:        "string",
				Description: "S3 bucket name (required if storage=s3)",
				Example:     "store-downloads",
				Validation:  "",
				Required:    false,
			},
			{
				Name:        "s3_region",
				Type:        "string",
				Description: "AWS region (required if storage=s3)",
				Example:     "us-east-1",
				Validation:  "",
				Required:    false,
			},
			{
				Name:        "s3_key_prefix",
				Type:        "string",
				Description: "Prefix for S3 object keys",
				Example:     "items/",
				Validation:  "",
				Required:    false,
			},
			{
				Name:        "expiration_hours",
				Type:        "number",
				Description: "Hours until download link expires",
				Example:     "24",
				Validation:  "^\\d+$",
				Required:    false,
			},
			{
				Name:        "max_downloads",
				Type:        "number",
				Description: "Maximum number of downloads allowed",
				Example:     "10",
				Validation:  "^\\d+$",
				Required:    false,
			},
		},
	}
}

// generateS3URL generates a presigned S3 URL for the item.
// In a real implementation, this would use the AWS SDK.
func (h *DigitalMediaHandler) generateS3URL(ctx context.Context, item *models.Item, config handler.Config) (string, error) {
	bucket := config.GetString("s3_bucket")
	region := config.GetString("s3_region")
	prefix := config.GetString("s3_key_prefix")

	if prefix == "" {
		prefix = "items/"
	}

	// In a real implementation, would use AWS SDK to generate presigned URL
	// For now, return a mock URL
	url := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s%s", bucket, region, prefix, item.ID)
	return url, nil
}
