resource "random_id" "bucket_suffix" {
  byte_length = 4
}

locals {
  media_bucket_name = var.media_bucket_name != "" ? var.media_bucket_name : "chatplatform-${var.environment}-media-${random_id.bucket_suffix.hex}"
}

resource "aws_s3_bucket" "media" {
  bucket = local.media_bucket_name
  tags   = { Name = "chatplatform-${var.environment}-media" }
}

resource "aws_s3_bucket_public_access_block" "media" {
  bucket                  = aws_s3_bucket.media.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_versioning" "media" {
  bucket = aws_s3_bucket.media.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "media" {
  bucket = aws_s3_bucket.media.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

# Presigned uploads (internal/media/s3.go) are issued directly from the
# backend to the client, so the bucket itself needs CORS for the
# browser/app's PUT — the backend never proxies the upload bytes.
resource "aws_s3_bucket_cors_configuration" "media" {
  bucket = aws_s3_bucket.media.id
  cors_rule {
    allowed_methods = ["GET", "PUT", "HEAD"]
    allowed_origins = ["*"] # tighten to the real Android app's nothing (native apps don't send Origin) plus any web admin origin once one exists
    allowed_headers = ["*"]
    expose_headers  = ["ETag"]
    max_age_seconds = 3600
  }
}

# Old, never-confirmed multipart uploads and soft-deleted media (the
# media module's own deleted_at-style lifecycle, if it adds one) are not
# automatically cleaned up by this bucket policy alone — this only
# aborts stale multipart uploads, which cost money even when they never
# complete.
resource "aws_s3_bucket_lifecycle_configuration" "media" {
  bucket = aws_s3_bucket.media.id
  rule {
    id     = "abort-incomplete-multipart-uploads"
    status = "Enabled"
    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }
}
