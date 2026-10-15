// Package channels implements Channels: one-way broadcast. Only admins
// post; followers read, react, and are counted as views. Distinct from
// chat.messages entirely — see the migration's header comment for why.
package channels

import "time"

const (
	PostText    = "text"
	PostImage   = "image"
	PostVideo   = "video"
	PostAudio   = "audio"
	PostSticker = "sticker"
	PostPoll    = "poll"
	PostLink    = "link"

	RoleOwner = "owner"
	RoleAdmin = "admin"
)

type Channel struct {
	ID          string
	Name        string
	Description *string
	IconMediaID *string
	Category    *string
	IsPublic    bool
	InviteCode  *string
	CreatedBy   string
	CreatedAt   time.Time
}

type PollOption struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Votes int64  `json:"votes"`
	// VotedByMe is only meaningful when the post is returned to a specific
	// follower (never set on the author-facing analytics view).
	VotedByMe bool `json:"voted_by_me"`
}

type Post struct {
	ID          string            `json:"id"`
	ChannelID   string            `json:"channel_id"`
	AuthorID    string            `json:"author_id"`
	Seq         int64             `json:"seq"`
	Type        string            `json:"type"`
	Body        *string           `json:"body,omitempty"`
	MediaID     *string           `json:"media_id,omitempty"`
	LinkURL     *string           `json:"link_url,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	EditedAt    *time.Time        `json:"edited_at,omitempty"`
	ViewCount   int64             `json:"view_count"`
	Reactions   []ReactionSummary `json:"reactions,omitempty"`
	PollOptions []PollOption      `json:"poll_options,omitempty"`
}

type ReactionSummary struct {
	Emoji string `json:"emoji"`
	Count int64  `json:"count"`
}

type Admin struct {
	UserID  string    `json:"user_id"`
	Role    string    `json:"role"`
	AddedAt time.Time `json:"added_at"`
}

// --- Request/response DTOs ---

type CreateChannelRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IconMediaID string `json:"icon_media_id,omitempty"`
	Category    string `json:"category,omitempty"`
	IsPublic    bool   `json:"is_public"`
}

type UpdateChannelRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	IconMediaID *string `json:"icon_media_id,omitempty"`
	Category    *string `json:"category,omitempty"`
	IsPublic    *bool   `json:"is_public,omitempty"`
}

type ChannelSummary struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Description   *string `json:"description,omitempty"`
	IconMediaID   *string `json:"icon_media_id,omitempty"`
	Category      *string `json:"category,omitempty"`
	IsPublic      bool    `json:"is_public"`
	FollowerCount int64   `json:"follower_count"`
	IsFollowing   bool    `json:"is_following"`
	IsAdmin       bool    `json:"is_admin"`
	InviteCode    *string `json:"invite_code,omitempty"` // admins/owner only
}

type CreatePostRequest struct {
	Type    string `json:"type"`
	Body    string `json:"body,omitempty"`
	MediaID string `json:"media_id,omitempty"`
	LinkURL string `json:"link_url,omitempty"`
	// PollOptions: required (2-10 entries) when Type == "poll".
	PollOptions []string `json:"poll_options,omitempty"`
}

type EditPostRequest struct {
	Body string `json:"body"`
}

type PostsResponse struct {
	Posts      []Post `json:"posts"`
	NextCursor string `json:"next_cursor,omitempty"`
}

type ReactRequest struct {
	Emoji string `json:"emoji"`
}

type VoteRequest struct {
	OptionID string `json:"option_id"`
}

type UpdateAdminRequest struct {
	Role string `json:"role"` // "admin" to add/keep, enforced owner-only to remove in the handler layer
}

type InviteLinkResponse struct {
	InviteCode string `json:"invite_code"`
}

type FollowResponse struct {
	ChannelID string `json:"channel_id"`
}

type ReportChannelRequest struct {
	Reason string `json:"reason"`
}

type SearchResponse struct {
	Channels []ChannelSummary `json:"channels"`
}
