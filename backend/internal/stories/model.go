// Package stories implements 24-hour Stories/Status: posting (text, image,
// video), per-story audience rules, mentions, views, reactions, replies,
// muting, and the owner's archive.
package stories

import "time"

const (
	TypeText  = "text"
	TypeImage = "image"
	TypeVideo = "video"

	PrivacyContacts       = "contacts"
	PrivacyContactsExcept = "contacts_except"
	PrivacySelected       = "selected"

	StoryLifetime = 24 * time.Hour
)

// Story is one story as returned to clients. Fields marked "own only" are
// left zero/omitted when someone else's story is returned.
type Story struct {
	ID              string    `json:"id"`
	AuthorID        string    `json:"author_id"`
	Type            string    `json:"type"`
	TextContent     *string   `json:"text_content,omitempty"`
	BackgroundColor *string   `json:"background_color,omitempty"`
	Caption         *string   `json:"caption,omitempty"`
	MediaID         *string   `json:"media_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	Seen            bool      `json:"seen"`                     // feed only
	Privacy         string    `json:"privacy,omitempty"`        // own only
	ViewCount       int64     `json:"view_count,omitempty"`     // own only
	ReactionCount   int64     `json:"reaction_count,omitempty"` // own only
	ReplyCount      int64     `json:"reply_count,omitempty"`    // own only
}

// FeedEntry groups one author's currently-visible stories.
type FeedEntry struct {
	AuthorID    string  `json:"author_id"`
	Username    string  `json:"username"`
	DisplayName *string `json:"display_name,omitempty"`
	Muted       bool    `json:"muted"`
	HasUnseen   bool    `json:"has_unseen"`
	Stories     []Story `json:"stories"` // oldest first, the order they play in
}

type FeedResponse struct {
	Entries []FeedEntry `json:"entries"`
}

type MyStoriesResponse struct {
	Stories []Story `json:"stories"`
}

type CreateStoryRequest struct {
	Type            string `json:"type"`
	TextContent     string `json:"text_content,omitempty"`
	BackgroundColor string `json:"background_color,omitempty"`
	Caption         string `json:"caption,omitempty"`
	MediaID         string `json:"media_id,omitempty"`
	Privacy         string `json:"privacy,omitempty"`
	// AudienceUserIDs: the excluded people for contacts_except, or the only
	// people who may see it for selected. Ignored for contacts.
	AudienceUserIDs []string `json:"audience_user_ids,omitempty"`
	// MentionUserIDs must all be the author's own registered contacts.
	MentionUserIDs []string `json:"mention_user_ids,omitempty"`
}

type Viewer struct {
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	DisplayName *string   `json:"display_name,omitempty"`
	ViewedAt    time.Time `json:"viewed_at"`
	Reaction    *string   `json:"reaction,omitempty"` // their emoji, if they reacted
}

type ViewersResponse struct {
	Viewers []Viewer `json:"viewers"`
}

type ReactRequest struct {
	Emoji string `json:"emoji"`
}

type ReplyRequest struct {
	Body string `json:"body"`
}

type Reply struct {
	ID        string    `json:"id"`
	StoryID   string    `json:"story_id"`
	ReplierID string    `json:"replier_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type RepliesResponse struct {
	Replies []Reply `json:"replies"`
}

// feedRow is one flat row from the feed query, before grouping by author.
type feedRow struct {
	Story       Story
	Username    string
	DisplayName *string
	Muted       bool
}
