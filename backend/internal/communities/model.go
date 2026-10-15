// Package communities implements Communities: a container for multiple
// existing groups plus one special announcement group every member is
// auto-joined to. Built on top of the chat package's groups (via small
// primitive-typed interfaces — see GroupCreator/GroupMembershipManager
// below) rather than a parallel membership system.
package communities

import "time"

type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

type Community struct {
	ID           string
	Name         string
	Description  *string
	IconMediaID  *string
	InviteCode   *string
	Discoverable bool
	CreatedBy    string
	CreatedAt    time.Time
}

type Member struct {
	UserID   string    `json:"user_id"`
	Role     Role      `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

type GroupRef struct {
	ConversationID string `json:"conversation_id"`
	Name           string `json:"name"`
	IsAnnouncement bool   `json:"is_announcement"`
}

// --- Request/response DTOs ---

type CreateCommunityRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	IconMediaID  string `json:"icon_media_id,omitempty"`
	Discoverable bool   `json:"discoverable,omitempty"`
}

type UpdateCommunityRequest struct {
	Name         *string `json:"name,omitempty"`
	Description  *string `json:"description,omitempty"`
	IconMediaID  *string `json:"icon_media_id,omitempty"`
	Discoverable *bool   `json:"discoverable,omitempty"`
}

type CreateGroupInCommunityRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type AddExistingGroupRequest struct {
	ConversationID string `json:"conversation_id"`
}

type UpdateMemberRoleRequest struct {
	Role Role `json:"role"`
}

type BanRequest struct {
	Reason string `json:"reason,omitempty"`
}

type ReportCommunityRequest struct {
	Reason string `json:"reason"`
}

type CommunitySummary struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Description  *string    `json:"description,omitempty"`
	IconMediaID  *string    `json:"icon_media_id,omitempty"`
	Discoverable bool       `json:"discoverable"`
	MemberCount  int64      `json:"member_count"`
	Groups       []GroupRef `json:"groups"`
	InviteCode   *string    `json:"invite_code,omitempty"` // handler only fills this in for admins/owner
}

type InviteLinkResponse struct {
	InviteCode string `json:"invite_code"`
}

type JoinResponse struct {
	CommunityID                string `json:"community_id"`
	AnnouncementConversationID string `json:"announcement_conversation_id,omitempty"`
}

type SearchResponse struct {
	Communities []CommunitySummary `json:"communities"`
}
