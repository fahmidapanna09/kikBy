package media

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrInvalidKind       = errors.New("media: invalid kind")
	ErrMimeNotAllowed    = errors.New("media: mime type not allowed for this kind")
	ErrNotUploadedYet    = errors.New("media: object has not been uploaded to storage yet")
	ErrSizeLimitExceeded = errors.New("media: file exceeds the size limit for its kind")
	ErrForbidden         = errors.New("media: not authorized to access this file")
)

// allowedMimeTypes is intentionally an allowlist, not a denylist — new file
// types must be explicitly added here rather than silently accepted.
var allowedMimeTypes = map[Kind]map[string]bool{
	KindImage: {"image/jpeg": true, "image/png": true, "image/webp": true, "image/gif": true},
	KindVideo: {"video/mp4": true, "video/webm": true, "video/3gpp": true},
	KindAudio: {"audio/mpeg": true, "audio/mp4": true, "audio/aac": true, "audio/webm": true},
	KindVoice: {"audio/aac": true, "audio/amr": true, "audio/ogg": true, "audio/webm": true, "audio/mp4": true},
	KindDocument: {
		"application/pdf": true, "application/msword": true,
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
		"text/plain": true, "application/zip": true,
	},
}

// MembershipChecker lets the media module verify "is this user in the
// conversation that this attachment was sent to" without importing the chat
// package directly — keeps the module boundary from architecture.md intact.
type MembershipChecker interface {
	IsMember(ctx context.Context, conversationID, userID string) (bool, error)
}

// StoryMediaChecker is the same idea for story images/videos: "can this
// user currently see the story this media belongs to". Satisfied
// structurally by stories.Repository.
type StoryMediaChecker interface {
	CanViewStoryMedia(ctx context.Context, viewerID, mediaID string) (bool, error)
}

type Limits struct {
	MaxImageBytes    int64
	MaxVideoBytes    int64
	MaxAudioBytes    int64
	MaxVoiceBytes    int64
	MaxDocumentBytes int64
}

func (l Limits) maxFor(kind Kind) int64 {
	switch kind {
	case KindImage:
		return l.MaxImageBytes
	case KindVideo:
		return l.MaxVideoBytes
	case KindAudio:
		return l.MaxAudioBytes
	case KindVoice:
		return l.MaxAudioBytes // voice notes share the audio limit
	case KindDocument:
		return l.MaxDocumentBytes
	default:
		return 0
	}
}

type Service struct {
	repo              *Repository
	s3                *S3Client
	membership        MembershipChecker
	storyMedia        StoryMediaChecker
	limits            Limits
	presignTTLSeconds int
}

func NewService(repo *Repository, s3client *S3Client, membership MembershipChecker, storyMedia StoryMediaChecker, limits Limits, presignTTLSeconds int) *Service {
	return &Service{repo: repo, s3: s3client, membership: membership, storyMedia: storyMedia, limits: limits, presignTTLSeconds: presignTTLSeconds}
}

// VerifyOwnedUploaded confirms mediaID belongs to userID and finished
// uploading, and reports its kind/mime — used by stories.Service so a story
// can't be posted pointing at someone else's file, an unconfirmed upload, or
// (via the kind check the caller does) the wrong media type for the story
// type it's attached to.
func (s *Service) VerifyOwnedUploaded(ctx context.Context, userID, mediaID string) (kind string, mimeType string, err error) {
	m, err := s.repo.GetByID(ctx, mediaID)
	if err != nil {
		return "", "", err
	}
	if m.OwnerID != userID {
		return "", "", ErrForbidden
	}
	if m.Status != StatusUploaded {
		return "", "", ErrNotUploadedYet
	}
	mt := m.DeclaredMimeType
	if m.ConfirmedMimeType != nil {
		mt = *m.ConfirmedMimeType
	}
	return string(m.Kind), mt, nil
}

// PresignUpload validates the declared kind/mime combination up front
// (cheap, catches obviously-wrong clients immediately) and creates a
// pending_upload row before handing back a presigned PUT URL. The row
// exists specifically so ConfirmUpload has something to validate against —
// an object key is never trusted just because a URL for it was issued.
func (s *Service) PresignUpload(ctx context.Context, userID string, req PresignUploadRequest) (PresignUploadResponse, error) {
	if !allowedMimeTypes[req.Kind][req.MimeType] {
		return PresignUploadResponse{}, ErrMimeNotAllowed
	}

	ext := path.Ext(req.FileName)
	if ext == "" {
		ext = extensionFor(req.MimeType)
	}
	objectKey := fmt.Sprintf("media/%s/%s%s", userID, uuid.NewString(), ext)

	m, err := s.repo.Create(ctx, userID, s.s3BucketPlaceholder(), objectKey, req.Kind, req.MimeType, SanitizeFileName(req.FileName))
	if err != nil {
		return PresignUploadResponse{}, fmt.Errorf("presign upload: create record: %w", err)
	}

	uploadURL, err := s.s3.PresignUpload(ctx, objectKey, req.MimeType)
	if err != nil {
		return PresignUploadResponse{}, err
	}

	return PresignUploadResponse{MediaID: m.ID, UploadURL: uploadURL, ExpiresInSeconds: s.presignTTLSeconds}, nil
}

// s3BucketPlaceholder exists because the bucket name is a property of the
// configured S3Client, not something the media table strictly needs to
// duplicate per-row — kept as a column (see migration) for the rare case a
// deployment shards across buckets later; for now every row uses the one
// configured bucket.
func (s *Service) s3BucketPlaceholder() string { return s.s3.bucket }

// ConfirmUpload is called by the client after it finishes the PUT to S3. It
// verifies via HEAD what was *actually* stored — size and content-type
// never come from client claims — and rejects (deleting the object) if the
// real content-type doesn't match what was declared/allowed, or if it
// exceeds the size limit for its kind.
func (s *Service) ConfirmUpload(ctx context.Context, userID, mediaID string) (ConfirmUploadResponse, error) {
	m, err := s.repo.GetByID(ctx, mediaID)
	if err != nil {
		return ConfirmUploadResponse{}, err
	}
	if m.OwnerID != userID {
		return ConfirmUploadResponse{}, ErrForbidden
	}

	head, err := s.s3.HeadObject(ctx, m.ObjectKey)
	if err != nil {
		return ConfirmUploadResponse{}, fmt.Errorf("confirm upload: head object: %w", err)
	}
	if !head.Exists {
		return ConfirmUploadResponse{}, ErrNotUploadedYet
	}

	maxBytes := s.limits.maxFor(m.Kind)
	if maxBytes > 0 && head.ContentLength > maxBytes {
		_ = s.s3.DeleteObject(ctx, m.ObjectKey)
		reason := fmt.Sprintf("file size %d bytes exceeds limit of %d bytes for %s", head.ContentLength, maxBytes, m.Kind)
		_ = s.repo.Reject(ctx, mediaID, reason)
		return ConfirmUploadResponse{MediaID: mediaID, Status: StatusRejected, Reason: reason}, nil
	}

	if !allowedMimeTypes[m.Kind][head.ContentType] {
		_ = s.s3.DeleteObject(ctx, m.ObjectKey)
		reason := fmt.Sprintf("uploaded content-type %q does not match an allowed type for %s", head.ContentType, m.Kind)
		_ = s.repo.Reject(ctx, mediaID, reason)
		return ConfirmUploadResponse{MediaID: mediaID, Status: StatusRejected, Reason: reason}, nil
	}

	if err := s.repo.ConfirmUploaded(ctx, mediaID, head.ContentType, head.ContentLength); err != nil {
		return ConfirmUploadResponse{}, err
	}

	downloadURL, err := s.s3.PresignDownload(ctx, m.ObjectKey)
	if err != nil {
		return ConfirmUploadResponse{}, err
	}

	return ConfirmUploadResponse{MediaID: mediaID, Status: StatusUploaded, DownloadURL: downloadURL}, nil
}

// PresignDownload authorizes on either of two grounds: the caller uploaded
// the file themselves, or the file is attached to a message in a
// conversation the caller is a member of. Anyone else gets ErrForbidden —
// media object keys are UUIDs and not guessable, but authorization is
// enforced regardless, not relied on as the only protection.
func (s *Service) PresignDownload(ctx context.Context, userID, mediaID string) (DownloadURLResponse, error) {
	return s.PresignDownloadVariant(ctx, userID, mediaID, false)
}

// PresignDownloadVariant is PresignDownload with an optional thumbnail: when thumb is true and the thumbnail worker has
// produced one, the link points at it (Variant "thumb"); otherwise it silently falls back to the full file (Variant "full").
func (s *Service) PresignDownloadVariant(ctx context.Context, userID, mediaID string, thumb bool) (DownloadURLResponse, error) {
	m, err := s.repo.GetByID(ctx, mediaID)
	if err != nil {
		return DownloadURLResponse{}, err
	}

	authorized := m.OwnerID == userID
	if !authorized {
		// A file can sit in several conversations once forwarded, so ask "is it in ANY conversation this user is in".
		authorized, err = s.repo.VisibleToMember(ctx, mediaID, userID)
		if err != nil {
			return DownloadURLResponse{}, err
		}
	}
	if !authorized && s.storyMedia != nil {
		authorized, err = s.storyMedia.CanViewStoryMedia(ctx, userID, mediaID)
		if err != nil {
			return DownloadURLResponse{}, err
		}
	}
	if !authorized {
		return DownloadURLResponse{}, ErrForbidden
	}
	if m.Status != StatusUploaded {
		return DownloadURLResponse{}, ErrNotUploadedYet
	}

	key, variant := m.ObjectKey, "full"
	if thumb && m.ThumbnailObjectKey != nil && *m.ThumbnailObjectKey != "" {
		key, variant = *m.ThumbnailObjectKey, "thumb"
	}
	url, err := s.s3.PresignDownload(ctx, key)
	if err != nil {
		return DownloadURLResponse{}, err
	}
	return DownloadURLResponse{DownloadURL: url, ExpiresInSeconds: s.presignTTLSeconds, Variant: variant}, nil
}

func (s *Service) conversationIDForMedia(ctx context.Context, mediaID string) (string, bool, error) {
	return s.repo.ConversationIDForMedia(ctx, mediaID)
}

func (s *Service) AttachToMessage(ctx context.Context, userID, messageID, mediaID string, position int) error {
	m, err := s.repo.GetByID(ctx, mediaID)
	if err != nil {
		return err
	}
	if m.OwnerID != userID {
		// Forwarding: allowed when the file is already in a conversation this user belongs to.
		ok, err := s.repo.VisibleToMember(ctx, mediaID, userID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrForbidden
		}
	}
	// Only files that passed confirm (size + real content-type checked) may
	// be attached; a pending or rejected upload never reaches a message.
	if m.Status != StatusUploaded {
		return ErrNotUploadedYet
	}
	return s.repo.AttachToMessage(ctx, messageID, mediaID, position)
}

func (s *Service) AttachmentsForMessage(ctx context.Context, messageID string) ([]MediaFile, error) {
	return s.repo.AttachmentsForMessage(ctx, messageID)
}

// DeleteMedia removes the object from storage and marks the row deleted
// (soft delete — the row stays for FK integrity, e.g. stories.media_id has
// no ON DELETE CASCADE, so callers like the stories cleanup worker can
// delete media before or after the thing that references it without a
// foreign-key error either way).
func (s *Service) DeleteMedia(ctx context.Context, mediaID string) error {
	m, err := s.repo.GetByID(ctx, mediaID)
	if errors.Is(err, ErrNotFound) {
		return nil // already gone — deleting is idempotent
	}
	if err != nil {
		return err
	}
	if err := s.s3.DeleteObject(ctx, m.ObjectKey); err != nil {
		return fmt.Errorf("delete media: remove object: %w", err)
	}
	if m.ThumbnailObjectKey != nil {
		_ = s.s3.DeleteObject(ctx, *m.ThumbnailObjectKey) // best-effort; a stray thumbnail costs little
	}
	return s.repo.MarkDeleted(ctx, mediaID)
}

func extensionFor(mimeType string) string {
	switch {
	case strings.Contains(mimeType, "jpeg"):
		return ".jpg"
	case strings.Contains(mimeType, "png"):
		return ".png"
	case strings.Contains(mimeType, "webp"):
		return ".webp"
	case strings.Contains(mimeType, "gif"):
		return ".gif"
	case strings.Contains(mimeType, "mp4"):
		return ".mp4"
	case strings.Contains(mimeType, "webm"):
		return ".webm"
	case strings.Contains(mimeType, "pdf"):
		return ".pdf"
	default:
		return ""
	}
}

// SanitizeFileName keeps only the base name, strips control characters and
// path separators, and caps the length. The result is display-only (never
// used as an object key).
func SanitizeFileName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(name)
	if name == "." || name == "/" {
		return ""
	}
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if rs := []rune(out); len(rs) > 120 {
		out = string(rs[:120])
	}
	return out
}

// PresignAvatar returns a short-lived download link for a profile picture. There is deliberately no per-viewer access
// check (profile pictures are visible to everyone, like in any chat app), so callers must pass a media id they read from
// users.avatar_media_id, never one supplied by a client. Only finished image uploads qualify.
func (s *Service) PresignAvatar(ctx context.Context, mediaID string) (string, error) {
	m, err := s.repo.GetByID(ctx, mediaID)
	if err != nil {
		return "", err
	}
	if m.Status != StatusUploaded || m.Kind != KindImage {
		return "", ErrNotUploadedYet
	}
	return s.s3.PresignDownload(ctx, m.ObjectKey)
}
