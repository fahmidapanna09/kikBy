package media

import "time"

type Kind string

const (
	KindImage    Kind = "image"
	KindVideo    Kind = "video"
	KindAudio    Kind = "audio"
	KindVoice    Kind = "voice"
	KindDocument Kind = "document"
)

type Status string

const (
	StatusPendingUpload Status = "pending_upload"
	StatusUploaded      Status = "uploaded"
	StatusRejected      Status = "rejected"
	StatusDeleted       Status = "deleted"
)

type MediaFile struct {
	ID                 string
	OwnerID            string
	Bucket             string
	ObjectKey          string
	Kind               Kind
	DeclaredMimeType   string
	ConfirmedMimeType  *string
	SizeBytes          *int64
	Width              *int
	Height             *int
	DurationMs         *int
	ThumbnailObjectKey *string
	Status             Status
	RejectionReason    *string
	CreatedAt          time.Time
}

// --- Request DTOs ---

type PresignUploadRequest struct {
	Kind     Kind   `json:"kind"`
	MimeType string `json:"mime_type"`
	FileName string `json:"file_name,omitempty"` // used only to derive an extension for the object key
}

type PresignUploadResponse struct {
	MediaID          string `json:"media_id"`
	UploadURL        string `json:"upload_url"`
	ExpiresInSeconds int    `json:"expires_in_seconds"`
}

type ConfirmUploadResponse struct {
	MediaID string `json:"media_id"`
	Status  Status `json:"status"`
	// DownloadURL is included on success so the client doesn't need a
	// second round trip immediately after confirming (e.g. to show its own
	// just-sent image before the message even round-trips).
	DownloadURL string `json:"download_url,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type DownloadURLResponse struct {
	DownloadURL      string `json:"download_url"`
	ExpiresInSeconds int    `json:"expires_in_seconds"`
	Variant          string `json:"variant,omitempty"` // "thumb" or "full"
}
