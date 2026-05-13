package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

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
		url, size, err := h.generateS3URLWithSize(ctx, item, config)
		if err != nil {
			return nil, err
		}
		downloadURL = url
		fileSize = size
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
				Name:        "s3_key",
				Type:        "string",
				Description: "Explicit S3 object key (overrides s3_key_prefix + item.ID)",
				Example:     "downloads/product.pdf",
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

// generateS3URLWithSize generates a presigned S3 URL and retrieves file size.
func (h *DigitalMediaHandler) generateS3URLWithSize(ctx context.Context, item *models.Item, config handler.Config) (string, int64, error) {
	bucket := config.GetString("s3_bucket")
	region := config.GetString("s3_region")
	key := getS3Key(item, config)
	expirationHours := getExpirationDuration(config)

	svc, err := createS3Session(region)
	if err != nil {
		return "", 0, err
	}

	fileSize, err := getObjectSize(ctx, svc, bucket, key)
	if err != nil {
		return "", 0, err
	}

	url, err := generatePresignedURL(svc, bucket, key, expirationHours)
	if err != nil {
		return "", 0, err
	}

	return url, fileSize, nil
}

// getS3Key constructs the S3 key from item and config.
func getS3Key(item *models.Item, config handler.Config) string {
	s3Key := config.GetString("s3_key")
	if s3Key != "" {
		return s3Key
	}

	prefix := config.GetString("s3_key_prefix")
	if prefix == "" {
		prefix = "items/"
	}

	return prefix + item.ID
}

// getExpirationDuration returns the expiration duration in hours from config.
func getExpirationDuration(config handler.Config) int {
	expirationHours := config.GetInt("expiration_hours")
	if expirationHours == 0 {
		return 24
	}
	return expirationHours
}

// createS3Session creates an AWS S3 service client.
func createS3Session(region string) (*s3.S3, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}
	return s3.New(sess), nil
}

// getObjectSize retrieves the file size from S3 object metadata.
func getObjectSize(ctx context.Context, svc *s3.S3, bucket, key string) (int64, error) {
	headOutput, err := svc.HeadObjectWithContext(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get object metadata: %w", err)
	}

	if headOutput.ContentLength != nil {
		return *headOutput.ContentLength, nil
	}

	return 0, nil
}

// generatePresignedURL generates a presigned S3 URL with the specified expiration.
func generatePresignedURL(svc *s3.S3, bucket, key string, expirationHours int) (string, error) {
	req, _ := svc.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})

	url, err := req.Presign(time.Duration(expirationHours) * time.Hour)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return url, nil
}

// generateS3URL generates a presigned S3 URL for the item (without size lookup).
func (h *DigitalMediaHandler) generateS3URL(ctx context.Context, item *models.Item, config handler.Config) (string, error) {
	url, _, err := h.generateS3URLWithSize(ctx, item, config)
	return url, err
}
