package test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/opd-ai/store/internal/handlers"
	"github.com/opd-ai/store/pkg/models"
)

const (
	minioEndpoint  = "http://localhost:9000"
	minioAccessKey = "minioadmin"
	minioSecretKey = "minioadmin"
	testBucket     = "test-store-bucket"
	testRegion     = "us-east-1"
)

// TestDigitalMediaHandler_S3_MinIO tests S3 integration using MinIO.
// This test requires MinIO to be running (e.g., via docker-compose).
// Skip if MINIO_ENDPOINT is not set or MinIO is not accessible.
func TestDigitalMediaHandler_S3_MinIO(t *testing.T) {
	// Check if MinIO is available
	endpoint := getEnvOrDefault("MINIO_ENDPOINT", minioEndpoint)
	accessKey := getEnvOrDefault("MINIO_ACCESS_KEY", minioAccessKey)
	secretKey := getEnvOrDefault("MINIO_SECRET_KEY", minioSecretKey)

	// Try to connect to MinIO
	if !isMinIOAvailable(endpoint) {
		t.Skip("MinIO not available, skipping integration test")
	}

	ctx := context.Background()

	// Create S3 client for MinIO
	sess, err := session.NewSession(&aws.Config{
		Endpoint:         aws.String(endpoint),
		Region:           aws.String(testRegion),
		Credentials:      credentials.NewStaticCredentials(accessKey, secretKey, ""),
		S3ForcePathStyle: aws.Bool(true), // Required for MinIO
	})
	if err != nil {
		t.Fatalf("Failed to create AWS session: %v", err)
	}

	svc := s3.New(sess)

	// Create test bucket
	if err := createBucketIfNotExists(ctx, svc, testBucket); err != nil {
		t.Fatalf("Failed to create bucket: %v", err)
	}

	// Cleanup: delete test objects after test
	defer cleanupTestObjects(ctx, svc, testBucket)

	// Upload a test file to MinIO
	testKey := "test-products/sample.pdf"
	testContent := []byte("This is a test PDF file content for digital media handler")
	if err := uploadTestFile(ctx, svc, testBucket, testKey, testContent); err != nil {
		t.Fatalf("Failed to upload test file: %v", err)
	}

	// Test the digital media handler with S3 storage
	handler := handlers.NewDigitalMediaHandler()

	item := &models.Item{
		ID:   "test-item-123",
		Name: "Test Digital Product",
		BackendConfig: models.JSONMap{
			"storage":          "s3",
			"s3_bucket":        testBucket,
			"s3_region":        testRegion,
			"s3_key":           testKey,
			"expiration_hours": 1,
			"max_downloads":    5,
		},
	}

	payment := &models.Payment{
		ID:     "test-payment-123",
		Status: "confirmed",
	}

	// Set AWS credentials as environment variables for the handler
	os.Setenv("AWS_ACCESS_KEY_ID", accessKey)
	os.Setenv("AWS_SECRET_ACCESS_KEY", secretKey)
	os.Setenv("AWS_ENDPOINT", endpoint)
	defer func() {
		os.Unsetenv("AWS_ACCESS_KEY_ID")
		os.Unsetenv("AWS_SECRET_ACCESS_KEY")
		os.Unsetenv("AWS_ENDPOINT")
	}()

	// Call the handler
	result, err := handler.Handle(ctx, payment, item)
	if err != nil {
		t.Fatalf("Handler.Handle() failed: %v", err)
	}

	// Verify result contains expected fields
	if result["download_url"] == nil {
		t.Error("Expected download_url in result")
	}

	downloadURL, ok := result["download_url"].(string)
	if !ok {
		t.Fatal("download_url is not a string")
	}

	t.Logf("Download URL: %s", downloadURL)

	if result["expires_at"] == nil {
		t.Error("Expected expires_at in result")
	}

	if result["file_size_mb"] == nil {
		t.Error("Expected file_size_mb in result")
	}

	fileSizeMB, ok := result["file_size_mb"].(int64)
	if !ok {
		t.Error("file_size_mb is not an int64")
	}

	// Verify file size is correct (should be 0 MB for our small test file)
	expectedSize := int64(len(testContent)) / (1024 * 1024)
	if fileSizeMB != expectedSize {
		t.Logf("File size: %d MB (expected %d MB for %d bytes)", fileSizeMB, expectedSize, len(testContent))
	}

	if result["max_downloads"] != 5 {
		t.Errorf("Expected max_downloads=5, got %v", result["max_downloads"])
	}
}

// TestDigitalMediaHandler_S3_InvalidCredentials tests error handling for invalid S3 credentials.
func TestDigitalMediaHandler_S3_InvalidCredentials(t *testing.T) {
	endpoint := getEnvOrDefault("MINIO_ENDPOINT", minioEndpoint)

	if !isMinIOAvailable(endpoint) {
		t.Skip("MinIO not available, skipping integration test")
	}

	ctx := context.Background()
	handler := handlers.NewDigitalMediaHandler()

	item := &models.Item{
		ID:   "test-item-456",
		Name: "Test Product",
		BackendConfig: models.JSONMap{
			"storage":          "s3",
			"s3_bucket":        "nonexistent-bucket",
			"s3_region":        testRegion,
			"s3_key":           "test.pdf",
			"expiration_hours": 1,
		},
	}

	payment := &models.Payment{
		ID:     "test-payment-456",
		Status: "confirmed",
	}

	// Set invalid credentials
	os.Setenv("AWS_ACCESS_KEY_ID", "invalid")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "invalid")
	os.Setenv("AWS_ENDPOINT", endpoint)
	defer func() {
		os.Unsetenv("AWS_ACCESS_KEY_ID")
		os.Unsetenv("AWS_SECRET_ACCESS_KEY")
		os.Unsetenv("AWS_ENDPOINT")
	}()

	// Call the handler - should fail with invalid credentials
	_, err := handler.Handle(ctx, payment, item)
	if err == nil {
		t.Error("Expected error with invalid credentials, got nil")
	}

	t.Logf("Got expected error: %v", err)
}

// isMinIOAvailable checks if MinIO is accessible at the given endpoint.
func isMinIOAvailable(endpoint string) bool {
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	// MinIO health check endpoint
	resp, err := client.Get(endpoint + "/minio/health/live")
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// createBucketIfNotExists creates a bucket if it doesn't already exist.
func createBucketIfNotExists(ctx context.Context, svc *s3.S3, bucket string) error {
	// Check if bucket exists
	_, err := svc.HeadBucketWithContext(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})
	if err == nil {
		// Bucket already exists
		return nil
	}

	// Create bucket
	_, err = svc.CreateBucketWithContext(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})
	return err
}

// uploadTestFile uploads a test file to S3/MinIO.
func uploadTestFile(ctx context.Context, svc *s3.S3, bucket, key string, content []byte) error {
	_, err := svc.PutObjectWithContext(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(content),
	})
	return err
}

// cleanupTestObjects deletes all test objects from the bucket.
func cleanupTestObjects(ctx context.Context, svc *s3.S3, bucket string) {
	// List all objects
	resp, err := svc.ListObjectsV2WithContext(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return
	}

	// Delete each object
	for _, obj := range resp.Contents {
		svc.DeleteObjectWithContext(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    obj.Key,
		})
	}
}

// TestMinIOPresignedURL tests that pre-signed URLs generated work correctly.
func TestMinIOPresignedURL(t *testing.T) {
	endpoint := getEnvOrDefault("MINIO_ENDPOINT", minioEndpoint)
	accessKey := getEnvOrDefault("MINIO_ACCESS_KEY", minioAccessKey)
	secretKey := getEnvOrDefault("MINIO_SECRET_KEY", minioSecretKey)

	if !isMinIOAvailable(endpoint) {
		t.Skip("MinIO not available, skipping integration test")
	}

	ctx := context.Background()

	// Create S3 client
	sess, err := session.NewSession(&aws.Config{
		Endpoint:         aws.String(endpoint),
		Region:           aws.String(testRegion),
		Credentials:      credentials.NewStaticCredentials(accessKey, secretKey, ""),
		S3ForcePathStyle: aws.Bool(true),
	})
	if err != nil {
		t.Fatalf("Failed to create AWS session: %v", err)
	}

	svc := s3.New(sess)

	// Create bucket and upload test file
	if err := createBucketIfNotExists(ctx, svc, testBucket); err != nil {
		t.Fatalf("Failed to create bucket: %v", err)
	}

	testKey := "downloads/presigned-test.txt"
	testContent := []byte("This content should be downloadable via pre-signed URL")
	if err := uploadTestFile(ctx, svc, testBucket, testKey, testContent); err != nil {
		t.Fatalf("Failed to upload test file: %v", err)
	}
	defer cleanupTestObjects(ctx, svc, testBucket)

	// Generate pre-signed URL
	req, _ := svc.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(testBucket),
		Key:    aws.String(testKey),
	})

	presignedURL, err := req.Presign(15 * time.Minute)
	if err != nil {
		t.Fatalf("Failed to generate pre-signed URL: %v", err)
	}

	t.Logf("Pre-signed URL: %s", presignedURL)

	// Download file using pre-signed URL
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(presignedURL)
	if err != nil {
		t.Fatalf("Failed to download file via pre-signed URL: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	// Read and verify content
	downloadedContent, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if !bytes.Equal(downloadedContent, testContent) {
		t.Errorf("Downloaded content doesn't match. Expected: %s, Got: %s", testContent, downloadedContent)
	}

	t.Log("Successfully downloaded file via pre-signed URL")
}

// getEnvOrDefault returns environment variable value or default if not set.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
