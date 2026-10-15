package chat

import "time"

type MemberRole string

const (
	RoleOwner  MemberRole = "owner"
	RoleAdmin  MemberRole = "admin"
	RoleMember MemberRole = "member"
)

type PermissionScope string

const (
	PermissionEveryone   PermissionScope = "everyone"
	PermissionAdminsOnly PermissionScope = "admins_only"
)

type Group struct {
	ConversationID   string
	Name             string
	Description      *string
	IconMediaID      *string
	WhoCanSend       PermissionScope
	WhoCanAddMembers PermissionScope
	WhoCanEditInfo   PermissionScope
	InviteCode       *string
	CreatedBy        string
	CreatedAt        time.Time
}

type Member struct {
	UserID   string     `json:"user_id"`
	Role     MemberRole `json:"role"`
	JoinedAt time.Time  `json:"joined_at"`
}

// --- Request/response DTOs ---

type CreateGroupRequest struct {
	Name          string   `json:"name"`
	Description   string   `json:"description,omitempty"`
	IconMediaID   string   `json:"icon_media_id,omitempty"`
	MemberUserIDs []string `json:"member_user_ids"`
}

type UpdateGroupRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	IconMediaID *string `json:"icon_media_id,omitempty"`
}

type AddMembersRequest struct {
	UserIDs []string `json:"user_ids"`
}

type UpdateMemberRoleRequest struct {
	Role MemberRole `json:"role"`
}

type GroupSummary struct {
	ConversationID   string          `json:"conversation_id"`
	Name             string          `json:"name"`
	Description      *string         `json:"description,omitempty"`
	IconMediaID      *string         `json:"icon_media_id,omitempty"`
	WhoCanSend       PermissionScope `json:"who_can_send"`
	WhoCanAddMembers PermissionScope `json:"who_can_add_members"`
	WhoCanEditInfo   PermissionScope `json:"who_can_edit_info"`
	Members          []Member        `json:"members"`
	InviteCode       *string         `json:"invite_code,omitempty"` // only populated for admins/owner in the handler
}

type InviteLinkResponse struct {
	InviteCode string `json:"invite_code"`
}

type JoinByInviteResponse struct {
	ConversationID string `json:"conversation_id"`
}

type ReactionRequest struct {
	Emoji string `json:"emoji"`
}

type ReactionSummary struct {
	Emoji string   `json:"emoji"`
	Users []string `json:"user_ids"`
}

type MuteRequest struct {
	// UntilRFC3339 empty/omitted = unmute. Otherwise a future timestamp;
	// "forever" is represented client-side as a far-future date, kept
	// simple here rather than adding a separate boolean.
	UntilRFC3339 string `json:"until,omitempty"`
}

type SearchResponse struct {
	Messages []Message `json:"messages"`
}
