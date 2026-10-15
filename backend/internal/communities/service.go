package communities

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrPermissionDenied    = errors.New("communities: insufficient permission for this action")
	ErrNotAMember          = errors.New("communities: you are not a member of this community")
	ErrInviteNotFound      = errors.New("communities: invite code not found or revoked")
	ErrBanned              = errors.New("communities: you have been banned from this community")
	ErrCannotBanOwner      = errors.New("communities: the owner cannot be banned or removed")
	ErrGroupNotOwnedByUser = errors.New("communities: you must be an admin or owner of that group to add it")
	ErrEmptyName           = errors.New("communities: name is required")
)

// GroupCreator is satisfied structurally by chat.Service — creating a group
// for a community (the announcement group, or a new sub-group) goes through
// the normal group-creation path so it behaves exactly like any other group.
type GroupCreator interface {
	CreateGroupWithSendPermission(ctx context.Context, creatorID, name, description string, adminsOnlySend bool) (conversationID string, err error)
}

// GroupMembershipManager covers the system-initiated membership changes a
// community needs to make on a group's behalf (auto-joining a new community
// member to the announcement group, dropping them on ban) without going
// through that group's own who_can_add_members permission check — the
// community's own permission check already gated the action.
type GroupMembershipManager interface {
	ForceAddMember(ctx context.Context, conversationID, userID string) error
	ForceRemoveMember(ctx context.Context, conversationID, userID string) error
}

// GroupMembershipChecker lets AddExistingGroup confirm the caller actually
// controls the group they're linking in, without re-implementing chat's
// role logic here.
type GroupMembershipChecker interface {
	MemberRoleOfString(ctx context.Context, conversationID, userID string) (role string, isMember bool, err error)
}

type Service struct {
	repo         *Repository
	groups       GroupCreator
	groupMembers GroupMembershipManager
	groupRoles   GroupMembershipChecker
}

func NewService(repo *Repository, groups GroupCreator, groupMembers GroupMembershipManager, groupRoles GroupMembershipChecker) *Service {
	return &Service{repo: repo, groups: groups, groupMembers: groupMembers, groupRoles: groupRoles}
}

func isAdminOrOwner(role Role) bool { return role == RoleOwner || role == RoleAdmin }

func (s *Service) CreateCommunity(ctx context.Context, creatorID string, req CreateCommunityRequest) (CommunitySummary, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return CommunitySummary{}, ErrEmptyName
	}
	req.Name = name

	c, err := s.repo.Create(ctx, creatorID, req)
	if err != nil {
		return CommunitySummary{}, err
	}
	if err := s.repo.AddMember(ctx, c.ID, creatorID, RoleOwner); err != nil {
		return CommunitySummary{}, err
	}

	// Every community gets an announcement group immediately — admins-only
	// posting, so it's the community's "broadcast" channel from the start.
	announcementConvID, err := s.groups.CreateGroupWithSendPermission(ctx, creatorID, name+" announcements", "", true)
	if err != nil {
		return CommunitySummary{}, err
	}
	if err := s.repo.LinkGroup(ctx, c.ID, announcementConvID, true); err != nil {
		return CommunitySummary{}, err
	}

	return s.summary(ctx, c, includeInvite(RoleOwner))
}

func (s *Service) Get(ctx context.Context, userID, communityID string) (CommunitySummary, error) {
	role, isMember, err := s.repo.MemberRoleOf(ctx, communityID, userID)
	if err != nil {
		return CommunitySummary{}, err
	}
	c, err := s.repo.Get(ctx, communityID)
	if err != nil {
		return CommunitySummary{}, err
	}
	if !isMember && !c.Discoverable {
		return CommunitySummary{}, ErrNotFound // don't reveal a private community's existence
	}
	return s.summary(ctx, c, includeInvite(role))
}

func includeInvite(role Role) bool { return isAdminOrOwner(role) }

func (s *Service) summary(ctx context.Context, c Community, showInvite bool) (CommunitySummary, error) {
	groups, err := s.repo.ListGroups(ctx, c.ID)
	if err != nil {
		return CommunitySummary{}, err
	}
	count, err := s.repo.MemberCount(ctx, c.ID)
	if err != nil {
		return CommunitySummary{}, err
	}
	sum := CommunitySummary{
		ID: c.ID, Name: c.Name, Description: c.Description, IconMediaID: c.IconMediaID,
		Discoverable: c.Discoverable, MemberCount: count, Groups: groups,
	}
	if showInvite {
		sum.InviteCode = c.InviteCode
	}
	return sum, nil
}

func (s *Service) Update(ctx context.Context, userID, communityID string, req UpdateCommunityRequest) (CommunitySummary, error) {
	role, isMember, err := s.requireMember(ctx, userID, communityID)
	if err != nil {
		return CommunitySummary{}, err
	}
	if !isAdminOrOwner(role) {
		return CommunitySummary{}, ErrPermissionDenied
	}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			return CommunitySummary{}, ErrEmptyName
		}
		req.Name = &trimmed
	}
	if err := s.repo.Update(ctx, communityID, req); err != nil {
		return CommunitySummary{}, err
	}
	c, err := s.repo.Get(ctx, communityID)
	if err != nil {
		return CommunitySummary{}, err
	}
	_ = isMember
	return s.summary(ctx, c, true)
}

func (s *Service) JoinByInvite(ctx context.Context, userID, inviteCode string) (JoinResponse, error) {
	c, err := s.repo.GetByInviteCode(ctx, inviteCode)
	if err != nil {
		return JoinResponse{}, err
	}
	return s.join(ctx, userID, c)
}

func (s *Service) JoinDiscoverable(ctx context.Context, userID, communityID string) (JoinResponse, error) {
	c, err := s.repo.Get(ctx, communityID)
	if err != nil {
		return JoinResponse{}, err
	}
	if !c.Discoverable {
		return JoinResponse{}, ErrNotFound
	}
	return s.join(ctx, userID, c)
}

func (s *Service) join(ctx context.Context, userID string, c Community) (JoinResponse, error) {
	if banned, err := s.repo.IsBanned(ctx, c.ID, userID); err != nil {
		return JoinResponse{}, err
	} else if banned {
		return JoinResponse{}, ErrBanned
	}
	if err := s.repo.AddMember(ctx, c.ID, userID, RoleMember); err != nil {
		return JoinResponse{}, err
	}

	resp := JoinResponse{CommunityID: c.ID}
	if announcementID, ok, err := s.repo.AnnouncementConversationID(ctx, c.ID); err == nil && ok {
		_ = s.groupMembers.ForceAddMember(ctx, announcementID, userID) // best-effort; membership row above is what matters most
		resp.AnnouncementConversationID = announcementID
	}
	return resp, nil
}

func (s *Service) Leave(ctx context.Context, userID, communityID string) error {
	role, isMember, err := s.requireMember(ctx, userID, communityID)
	if err != nil {
		return err
	}
	if role == RoleOwner {
		return errors.New("communities: the owner can't leave — transfer ownership isn't supported yet")
	}
	_ = isMember
	if announcementID, ok, err := s.repo.AnnouncementConversationID(ctx, communityID); err == nil && ok {
		_ = s.groupMembers.ForceRemoveMember(ctx, announcementID, userID)
	}
	return s.repo.RemoveMember(ctx, communityID, userID)
}

func (s *Service) ListMembers(ctx context.Context, userID, communityID string) ([]Member, error) {
	if _, _, err := s.requireMember(ctx, userID, communityID); err != nil {
		return nil, err
	}
	return s.repo.ListMembers(ctx, communityID)
}

func (s *Service) UpdateMemberRole(ctx context.Context, actingUserID, communityID, targetUserID string, newRole Role) error {
	actingRole, _, err := s.requireMember(ctx, actingUserID, communityID)
	if err != nil {
		return err
	}
	if newRole == RoleOwner {
		return ErrPermissionDenied // ownership transfer isn't supported in Phase 7
	}
	targetRole, targetIsMember, err := s.repo.MemberRoleOf(ctx, communityID, targetUserID)
	if err != nil {
		return err
	}
	if !targetIsMember {
		return ErrNotAMember
	}
	switch {
	case newRole == RoleAdmin && targetRole == RoleMember:
		if !isAdminOrOwner(actingRole) {
			return ErrPermissionDenied
		}
	case newRole == RoleMember && targetRole == RoleAdmin:
		if actingRole != RoleOwner {
			return ErrPermissionDenied
		}
	default:
		return ErrPermissionDenied
	}
	return s.repo.UpdateMemberRole(ctx, communityID, targetUserID, newRole)
}

// RemoveMember is a plain removal (the person can rejoin later, e.g. via
// invite link); Ban additionally blocks rejoining.
func (s *Service) RemoveMember(ctx context.Context, actingUserID, communityID, targetUserID string) error {
	actingRole, err := s.requireManagerNotTargetingOwner(ctx, actingUserID, communityID, targetUserID)
	if err != nil {
		return err
	}
	_ = actingRole
	if announcementID, ok, err := s.repo.AnnouncementConversationID(ctx, communityID); err == nil && ok {
		_ = s.groupMembers.ForceRemoveMember(ctx, announcementID, targetUserID)
	}
	return s.repo.RemoveMember(ctx, communityID, targetUserID)
}

func (s *Service) Ban(ctx context.Context, actingUserID, communityID, targetUserID string) error {
	if _, err := s.requireManagerNotTargetingOwner(ctx, actingUserID, communityID, targetUserID); err != nil {
		return err
	}
	if announcementID, ok, err := s.repo.AnnouncementConversationID(ctx, communityID); err == nil && ok {
		_ = s.groupMembers.ForceRemoveMember(ctx, announcementID, targetUserID)
	}
	return s.repo.Ban(ctx, communityID, targetUserID, actingUserID)
}

func (s *Service) Unban(ctx context.Context, actingUserID, communityID, targetUserID string) error {
	role, _, err := s.requireMember(ctx, actingUserID, communityID)
	if err != nil {
		return err
	}
	if !isAdminOrOwner(role) {
		return ErrPermissionDenied
	}
	return s.repo.Unban(ctx, communityID, targetUserID)
}

// requireManagerNotTargetingOwner is the shared guard for RemoveMember/Ban:
// the actor must be an admin/owner, and the target must not be the owner
// (an owner can only leave on their own, once ownership transfer exists).
func (s *Service) requireManagerNotTargetingOwner(ctx context.Context, actingUserID, communityID, targetUserID string) (Role, error) {
	actingRole, _, err := s.requireMember(ctx, actingUserID, communityID)
	if err != nil {
		return "", err
	}
	if !isAdminOrOwner(actingRole) {
		return "", ErrPermissionDenied
	}
	targetRole, targetIsMember, err := s.repo.MemberRoleOf(ctx, communityID, targetUserID)
	if err != nil {
		return "", err
	}
	if targetIsMember && targetRole == RoleOwner {
		return "", ErrCannotBanOwner
	}
	return actingRole, nil
}

// --- Groups within a community ---

func (s *Service) CreateGroupInCommunity(ctx context.Context, userID, communityID string, req CreateGroupInCommunityRequest) (GroupRef, error) {
	role, _, err := s.requireMember(ctx, userID, communityID)
	if err != nil {
		return GroupRef{}, err
	}
	if !isAdminOrOwner(role) {
		return GroupRef{}, ErrPermissionDenied
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return GroupRef{}, ErrEmptyName
	}
	conversationID, err := s.groups.CreateGroupWithSendPermission(ctx, userID, name, req.Description, false)
	if err != nil {
		return GroupRef{}, err
	}
	if err := s.repo.LinkGroup(ctx, communityID, conversationID, false); err != nil {
		return GroupRef{}, err
	}
	return GroupRef{ConversationID: conversationID, Name: name}, nil
}

// AddExistingGroup requires the caller to be an admin/owner of BOTH the
// community and the group being linked — a community admin can't pull in
// someone else's group without also controlling it.
func (s *Service) AddExistingGroup(ctx context.Context, userID, communityID, conversationID string) error {
	communityRole, _, err := s.requireMember(ctx, userID, communityID)
	if err != nil {
		return err
	}
	if !isAdminOrOwner(communityRole) {
		return ErrPermissionDenied
	}
	groupRoleStr, groupIsMember, err := s.groupRoles.MemberRoleOfString(ctx, conversationID, userID)
	if err != nil {
		return err
	}
	if !groupIsMember || (groupRoleStr != string(RoleOwner) && groupRoleStr != string(RoleAdmin)) {
		return ErrGroupNotOwnedByUser
	}
	return s.repo.LinkGroup(ctx, communityID, conversationID, false)
}

func (s *Service) RemoveGroup(ctx context.Context, userID, communityID, conversationID string) error {
	role, _, err := s.requireMember(ctx, userID, communityID)
	if err != nil {
		return err
	}
	if !isAdminOrOwner(role) {
		return ErrPermissionDenied
	}
	return s.repo.UnlinkGroup(ctx, communityID, conversationID)
}

// --- Invite links ---

func (s *Service) GenerateInviteLink(ctx context.Context, userID, communityID string) (string, error) {
	role, _, err := s.requireMember(ctx, userID, communityID)
	if err != nil {
		return "", err
	}
	if !isAdminOrOwner(role) {
		return "", ErrPermissionDenied
	}
	return s.repo.SetNewInviteCode(ctx, communityID)
}

func (s *Service) RevokeInviteLink(ctx context.Context, userID, communityID string) error {
	role, _, err := s.requireMember(ctx, userID, communityID)
	if err != nil {
		return err
	}
	if !isAdminOrOwner(role) {
		return ErrPermissionDenied
	}
	return s.repo.RevokeInviteCode(ctx, communityID)
}

// --- Search & reports ---

// Mine lists every community the caller is a member of, each with its
// invite code included where the caller is an admin/owner (matching Get's
// own visibility rule for that field).
func (s *Service) Mine(ctx context.Context, userID string) (SearchResponse, error) {
	list, err := s.repo.ListForUser(ctx, userID)
	if err != nil {
		return SearchResponse{}, err
	}
	out := make([]CommunitySummary, 0, len(list))
	for _, c := range list {
		role, _, err := s.repo.MemberRoleOf(ctx, c.ID, userID)
		if err != nil {
			return SearchResponse{}, err
		}
		sum, err := s.summary(ctx, c, includeInvite(role))
		if err != nil {
			return SearchResponse{}, err
		}
		out = append(out, sum)
	}
	return SearchResponse{Communities: out}, nil
}

func (s *Service) Search(ctx context.Context, query string) (SearchResponse, error) {
	list, err := s.repo.SearchDiscoverable(ctx, strings.TrimSpace(query), 20)
	if err != nil {
		return SearchResponse{}, err
	}
	out := make([]CommunitySummary, 0, len(list))
	for _, c := range list {
		sum, err := s.summary(ctx, c, false)
		if err != nil {
			return SearchResponse{}, err
		}
		out = append(out, sum)
	}
	return SearchResponse{Communities: out}, nil
}

func (s *Service) Report(ctx context.Context, reporterID, communityID, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("communities: reason is required")
	}
	return s.repo.Report(ctx, reporterID, communityID, reason)
}

func (s *Service) requireMember(ctx context.Context, userID, communityID string) (Role, bool, error) {
	role, isMember, err := s.repo.MemberRoleOf(ctx, communityID, userID)
	if err != nil {
		return "", false, err
	}
	if !isMember {
		return "", false, ErrNotAMember
	}
	return role, true, nil
}
