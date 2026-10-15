package media

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("media: not found")

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, ownerID, bucket, objectKey string, kind Kind, declaredMime, originalName string) (MediaFile, error) {
	var m MediaFile
	err := r.pool.QueryRow(ctx, `
		INSERT INTO media_files (owner_id, bucket, object_key, kind, declared_mime_type, original_name)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''))
		RETURNING id, owner_id, bucket, object_key, kind, declared_mime_type, status, created_at
	`, ownerID, bucket, objectKey, string(kind), declaredMime, originalName).Scan(
		&m.ID, &m.OwnerID, &m.Bucket, &m.ObjectKey, &m.Kind, &m.DeclaredMimeType, &m.Status, &m.CreatedAt,
	)
	return m, err
}

func (r *Repository) GetByID(ctx context.Context, id string) (MediaFile, error) {
	var m MediaFile
	err := r.pool.QueryRow(ctx, `
		SELECT id, owner_id, bucket, object_key, kind, declared_mime_type, confirmed_mime_type,
		       size_bytes, width, height, duration_ms, thumbnail_object_key, status, rejection_reason, created_at
		FROM media_files WHERE id = $1 AND deleted_at IS NULL
	`, id).Scan(
		&m.ID, &m.OwnerID, &m.Bucket, &m.ObjectKey, &m.Kind, &m.DeclaredMimeType, &m.ConfirmedMimeType,
		&m.SizeBytes, &m.Width, &m.Height, &m.DurationMs, &m.ThumbnailObjectKey, &m.Status, &m.RejectionReason, &m.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return MediaFile{}, ErrNotFound
	}
	return m, err
}

func (r *Repository) ConfirmUploaded(ctx context.Context, id, confirmedMimeType string, sizeBytes int64) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE media_files
		SET status = 'uploaded', confirmed_mime_type = $2, size_bytes = $3, confirmed_at = $4
		WHERE id = $1
	`, id, confirmedMimeType, sizeBytes, time.Now().UTC())
	return err
}

func (r *Repository) MarkDeleted(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE media_files SET status = 'deleted' WHERE id = $1`, id)
	return err
}

func (r *Repository) Reject(ctx context.Context, id, reason string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE media_files SET status = 'rejected', rejection_reason = $2 WHERE id = $1`,
		id, reason,
	)
	return err
}

func (r *Repository) SetDimensions(ctx context.Context, id string, width, height, durationMs *int) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE media_files SET width = $2, height = $3, duration_ms = $4 WHERE id = $1`,
		id, width, height, durationMs,
	)
	return err
}

func (r *Repository) SetThumbnail(ctx context.Context, id, thumbnailObjectKey string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE media_files SET thumbnail_object_key = $2 WHERE id = $1`,
		id, thumbnailObjectKey,
	)
	return err
}

// IsOwner is the object-level authorization check used before issuing a
// download URL or attaching this media to a message.
func (r *Repository) IsOwner(ctx context.Context, mediaID, userID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM media_files WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL)`,
		mediaID, userID,
	).Scan(&exists)
	return exists, err
}

// AttachToMessage links already-uploaded media to a message. Called from
// within the same transaction as message insertion in a full
// implementation; kept as its own statement here since chat.Repository and
// media.Repository are separate modules by design (see architecture doc's
// module-boundary rule) — the chat service calls both under one request,
// not one shared DB transaction, which is an acceptable tradeoff since a
// half-attached message is self-healing (client can retry attaching) rather
// than corrupting.
func (r *Repository) AttachToMessage(ctx context.Context, messageID, mediaID string, position int) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO message_attachments (message_id, media_id, position) VALUES ($1, $2, $3)
		 ON CONFLICT (message_id, media_id) DO NOTHING`,
		messageID, mediaID, position,
	)
	return err
}

func (r *Repository) AttachmentsForMessage(ctx context.Context, messageID string) ([]MediaFile, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT m.id, m.owner_id, m.bucket, m.object_key, m.kind, m.declared_mime_type, m.confirmed_mime_type,
		       m.size_bytes, m.width, m.height, m.duration_ms, m.thumbnail_object_key, m.status, m.rejection_reason, m.created_at
		FROM message_attachments a
		JOIN media_files m ON m.id = a.media_id
		WHERE a.message_id = $1
		ORDER BY a.position ASC
	`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MediaFile
	for rows.Next() {
		var m MediaFile
		if err := rows.Scan(
			&m.ID, &m.OwnerID, &m.Bucket, &m.ObjectKey, &m.Kind, &m.DeclaredMimeType, &m.ConfirmedMimeType,
			&m.SizeBytes, &m.Width, &m.Height, &m.DurationMs, &m.ThumbnailObjectKey, &m.Status, &m.RejectionReason, &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ConversationIDForMedia returns the conversation a piece of media was sent
// into, if it has been attached to a message yet (a freshly-uploaded,
// not-yet-sent attachment has no conversation and returns attached=false).
func (r *Repository) ConversationIDForMedia(ctx context.Context, mediaID string) (conversationID string, attached bool, err error) {
	err = r.pool.QueryRow(ctx, `
		SELECT m.conversation_id
		FROM message_attachments a
		JOIN messages m ON m.id = a.message_id
		WHERE a.media_id = $1
		LIMIT 1
	`, mediaID).Scan(&conversationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return conversationID, true, nil
}

// PendingThumbnails is polled by the thumbnail worker.
func (r *Repository) PendingThumbnails(ctx context.Context, limit int) ([]MediaFile, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, owner_id, bucket, object_key, kind, declared_mime_type, confirmed_mime_type,
		       size_bytes, width, height, duration_ms, thumbnail_object_key, status, rejection_reason, created_at
		FROM media_files
		WHERE status = 'uploaded' AND thumbnail_object_key IS NULL AND kind = 'image'
		ORDER BY created_at
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MediaFile
	for rows.Next() {
		var m MediaFile
		if err := rows.Scan(
			&m.ID, &m.OwnerID, &m.Bucket, &m.ObjectKey, &m.Kind, &m.DeclaredMimeType, &m.ConfirmedMimeType,
			&m.SizeBytes, &m.Width, &m.Height, &m.DurationMs, &m.ThumbnailObjectKey, &m.Status, &m.RejectionReason, &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// VisibleToMember reports whether the media is attached to a (not deleted-for-everyone) message in any conversation the
// user is currently a member of.
func (r *Repository) VisibleToMember(ctx context.Context, mediaID, userID string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM message_attachments a
		  JOIN messages m ON m.id = a.message_id AND m.deleted_for_everyone_at IS NULL
		  JOIN conversation_members cm ON cm.conversation_id = m.conversation_id AND cm.user_id = $2 AND cm.left_at IS NULL
		  WHERE a.media_id = $1)`, mediaID, userID).Scan(&ok)
	return ok, err
}
