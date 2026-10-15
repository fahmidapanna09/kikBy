// Package media implements object-storage-backed media: presigned
// upload/download, upload confirmation (via S3 HEAD, never trusting the
// client's claims about what it actually uploaded), and thumbnail
// generation. Works against real AWS S3 or any S3-compatible store
// (MinIO for local dev) via the same client, switched by config only.
package media

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Client struct {
	client  *s3.Client        // used for HeadObject/PutObject/GetObject/Delete — safe to hit over the internal docker network
	presign *s3.PresignClient // built against the *public* endpoint, since the resulting URL is handed to a client outside the backend's network
	bucket  string
	ttl     time.Duration
}

type S3Config struct {
	Endpoint        string // internal endpoint the backend itself uses, e.g. "http://minio:9000"
	PresignEndpoint string // endpoint embedded into presigned URLs handed to clients, e.g. "http://localhost:9000" or "http://10.0.2.2:9000" for the Android emulator. Empty = same as Endpoint (correct for real AWS S3, where there's only one public endpoint).
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	UsePathStyle    bool
	PresignTTL      time.Duration
}

func NewS3Client(ctx context.Context, cfg S3Config) (*S3Client, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("media: load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.UsePathStyle
	})

	publicEndpoint := cfg.PresignEndpoint
	if publicEndpoint == "" {
		publicEndpoint = cfg.Endpoint
	}
	presignSourceClient := client
	if publicEndpoint != cfg.Endpoint {
		presignSourceClient = s3.NewFromConfig(awsCfg, func(o *s3.Options) {
			if publicEndpoint != "" {
				o.BaseEndpoint = aws.String(publicEndpoint)
			}
			o.UsePathStyle = cfg.UsePathStyle
		})
	}

	return &S3Client{
		client:  client,
		presign: s3.NewPresignClient(presignSourceClient),
		bucket:  cfg.Bucket,
		ttl:     cfg.PresignTTL,
	}, nil
}

// PresignUpload returns a URL the client PUTs the raw file bytes to
// directly — the file never passes through our own backend, keeping large
// uploads off the API's request-handling path entirely.
func (s *S3Client) PresignUpload(ctx context.Context, objectKey, contentType string) (string, error) {
	req, err := s.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(objectKey),
		ContentType: aws.String(contentType),
	}, s3.WithPresignExpires(s.ttl))
	if err != nil {
		return "", fmt.Errorf("media: presign upload: %w", err)
	}
	return req.URL, nil
}

func (s *S3Client) PresignDownload(ctx context.Context, objectKey string) (string, error) {
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(objectKey),
	}, s3.WithPresignExpires(s.ttl))
	if err != nil {
		return "", fmt.Errorf("media: presign download: %w", err)
	}
	return req.URL, nil
}

type HeadResult struct {
	ContentType   string
	ContentLength int64
	Exists        bool
}

// HeadObject is how we confirm what was *actually* uploaded — size and
// content-type come from S3 itself, never from client-supplied metadata,
// which is what makes the size-limit/MIME validation trustworthy.
func (s *S3Client) HeadObject(ctx context.Context, objectKey string) (HeadResult, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		// A 404 here just means "not uploaded yet" — the caller (service.go
		// ConfirmUpload) is expected to treat this as "not found" rather
		// than a hard error and prompt the client to retry the upload.
		return HeadResult{Exists: false}, nil
	}
	ct := ""
	if out.ContentType != nil {
		ct = *out.ContentType
	}
	length := int64(0)
	if out.ContentLength != nil {
		length = *out.ContentLength
	}
	return HeadResult{ContentType: ct, ContentLength: length, Exists: true}, nil
}

func (s *S3Client) DeleteObject(ctx context.Context, objectKey string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(objectKey),
	})
	return err
}

func (s *S3Client) GetObjectBytes(ctx context.Context, objectKey string) ([]byte, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()
	buf := make([]byte, 0, 64*1024)
	tmp := make([]byte, 32*1024)
	for {
		n, readErr := out.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if readErr != nil {
			break
		}
	}
	return buf, nil
}

func (s *S3Client) PutObjectBytes(ctx context.Context, objectKey, contentType string, data []byte) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(objectKey),
		ContentType: aws.String(contentType),
		Body:        bytes.NewReader(data),
	})
	return err
}
