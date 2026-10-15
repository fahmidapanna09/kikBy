package chat

import (
	"context"
	"time"
)

func (s *Service) CreateGroup(ctx context.Context, creatorID string, req CreateGroupRequest) (GroupSummary, error) {
	g, err := s.repo.CreateGroup(ctx, creatorID, req)
	if err != nil {
		return GroupSummary{}, err
	}
	summary, err := s.groupSummary(ctx, g)
	if err != nil {
		return GroupSummary{}, err
	}
	// Tell every member (not just the creator) a group now exists, so their
	// clients refresh the conversation list and the group shows up without
	// waiting for a manual refresh.
	s.broadcastToMembers(ctx, g.ConversationID, NewEnvelope(EventGroupUpdated, g.ConversationID, "", GroupUpdatedPayload{Group: summary}))
	return summary, nil
}

func (s *Service) GetGroup(ctx context.Context, userID, conversationID string) (GroupSummary, error) {
	_, isMember, err := s.repo.MemberRoleOf(ctx, conversationID, userID)
	if err != nil {
		return GroupSummary{}, err
	}
	if !isMember {
		return GroupSummary{}, ErrNotAMember
	}
	g, err := s.repo.GetGroup(ctx, conversationID)
	if err != nil {
		return GroupSummary{}, err
	}
	return s.groupSummary(ctx, g)
}

func (s *Service) groupSummary(ctx context.Context, g Group) (GroupSummary, error) {
	members, err := s.repo.ListMembers(ctx, g.ConversationID)
	if err != nil {
		return GroupSummary{}, err
	}
	return GroupSummary{
		ConversationID: g.ConversationID, Name: g.Name, Description: g.Description, IconMediaID: g.IconMediaID,
		WhoCanSend: g.WhoCanSend, WhoCanAddMembers: g.WhoCanAddMembers, WhoCanEditInfo: g.WhoCanEditInfo,
		Members: members,
		// InviteCode deliberately omitted here — only returned to
		// admins/owner explicitly via GenerateInviteLink/GetGroup's caller
		// checking role, so a plain member listing a group's info can't
		// read out an active invite link they weren't given.
	}, nil
}

// CreateGroupWithSendPermission is a thin adapter for callers outside this
// package (e.g. communities.Service, for a community's announcement group)
// that need to set who_can_send at creation time without depending on
// chat's CreateGroupRequest type — keeps that caller's interface
// primitive-typed, same module-boundary approach as AttachmentLinker/
// BlockChecker. Returns the new group's conversation id.
func (s *Service) CreateGroupWithSendPermission(ctx context.Context, creatorID, name, description string, adminsOnlySend bool) (string, error) {
	g, err := s.repo.CreateGroup(ctx, creatorID, CreateGroupRequest{Name: name, Description: description})
	if err != nil {
		return "", err
	}
	if adminsOnlySend {
		if err := s.repo.SetWhoCanSend(ctx, g.ConversationID, PermissionAdminsOnly); err != nil {
			return "", err
		}
	}
	return g.ConversationID, nil
}

// ForceAddMember and ForceRemoveMember bypass the group's who_can_send/
// who_can_add_members permission checks entirely — for system-initiated
// membership changes where authorization already happened one level up
// (e.g. communities.Service already confirmed the caller may manage this
// community before asking to add/remove someone from its announcement
// group). Never expose these directly as a public "manage this group" API;
// AddMembers/RemoveMember (permission-checked) are what that's for.
func (s *Service) ForceAddMember(ctx context.Context, conversationID, userID string) error {
	return s.repo.AddMembers(ctx, conversationID, []string{userID})
}

func (s *Service) ForceRemoveMember(ctx context.Context, conversationID, userID string) error {
	return s.repo.RemoveMember(ctx, conversationID, userID)
}

func isAdminOrOwner(role MemberRole) bool { return role == RoleOwner || role == RoleAdmin }

func (s *Service) UpdateGroup(ctx context.Context, userID, conversationID string, req UpdateGroupRequest) (GroupSummary, error) {
	role, isMember, err := s.repo.MemberRoleOf(ctx, conversationID, userID)
	if err != nil {
		return GroupSummary{}, err
	}
	if !isMember {
		return GroupSummary{}, ErrNotAMember
	}
	g, err := s.repo.GetGroup(ctx, conversationID)
	if err != nil {
		return GroupSummary{}, err
	}
	if g.WhoCanEditInfo == PermissionAdminsOnly && !isAdminOrOwner(role) {
		return GroupSummary{}, ErrPermissionDenied
	}

	if err := s.repo.UpdateGroup(ctx, conversationID, req); err != nil {
		return GroupSummary{}, err
	}
	updated, err := s.repo.GetGroup(ctx, conversationID)
	if err != nil {
		return GroupSummary{}, err
	}
	summary, err := s.groupSummary(ctx, updated)
	if err != nil {
		return GroupSummary{}, err
	}

	s.broadcastToMembers(ctx, conversationID, NewEnvelope(EventGroupUpdated, conversationID, "", GroupUpdatedPayload{Group: summary}))
	return summary, nil
}

func (s *Service) AddMembers(ctx context.Context, actingUserID, conversationID string, userIDs []string) error {
	role, isMember, err := s.repo.MemberRoleOf(ctx, conversationID, actingUserID)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrNotAMember
	}
	g, err := s.repo.GetGroup(ctx, conversationID)
	if err != nil {
		return err
	}
	if g.WhoCanAddMembers == PermissionAdminsOnly && !isAdminOrOwner(role) {
		return ErrPermissionDenied
	}

	if err := s.repo.AddMembers(ctx, conversationID, userIDs); err != nil {
		return err
	}

	added := make([]Member, 0, len(userIDs))
	for _, uid := range userIDs {
		added = append(added, Member{UserID: uid, Role: RoleMember, JoinedAt: time.Now().UTC()})
	}
	s.broadcastToMembers(ctx, conversationID, NewEnvelope(EventGroupMemberAdded, conversationID, "", GroupMemberAddedPayload{Members: added, AddedBy: actingUserID}))
	return nil
}

// RemoveMember covers both "leave" (targetUserID == actingUserID, always
// allowed) and "remove someone else" (requires admin/owner, and a
// non-owner can never remove an owner).
func (s *Service) RemoveMember(ctx context.Context, actingUserID, conversationID, targetUserID string) error {
	actingRole, isMember, err := s.repo.MemberRoleOf(ctx, conversationID, actingUserID)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrNotAMember
	}

	if targetUserID != actingUserID {
		if !isAdminOrOwner(actingRole) {
			return ErrPermissionDenied
		}
		targetRole, targetIsMember, err := s.repo.MemberRoleOf(ctx, conversationID, targetUserID)
		if err != nil {
			return err
		}
		if targetIsMember && targetRole == RoleOwner {
			return ErrPermissionDenied // owners can only be removed by leaving on their own
		}
	}

	if err := s.repo.RemoveMember(ctx, conversationID, targetUserID); err != nil {
		return err
	}
	s.broadcastToMembers(ctx, conversationID, NewEnvelope(EventGroupMemberRemoved, conversationID, "", GroupMemberRemovedPayload{UserID: targetUserID, RemovedBy: actingUserID}))
	return nil
}

// UpdateMemberRole: owner or admin can promote a member to admin; only the
// owner can demote an admin back to member (an admin can't de-admin a
// fellow admin, which would otherwise let admins lock each other out).
func (s *Service) UpdateMemberRole(ctx context.Context, actingUserID, conversationID, targetUserID string, newRole MemberRole) error {
	actingRole, isMember, err := s.repo.MemberRoleOf(ctx, conversationID, actingUserID)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrNotAMember
	}
	if newRole == RoleOwner {
		return ErrPermissionDenied // ownership transfer isn't supported in Phase 4
	}

	targetRole, targetIsMember, err := s.repo.MemberRoleOf(ctx, conversationID, targetUserID)
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

	if err := s.repo.UpdateMemberRole(ctx, conversationID, targetUserID, newRole); err != nil {
		return err
	}
	summary, err := s.GetGroup(ctx, actingUserID, conversationID)
	if err != nil {
		return err
	}
	s.broadcastToMembers(ctx, conversationID, NewEnvelope(EventGroupUpdated, conversationID, "", GroupUpdatedPayload{Group: summary}))
	return nil
}

func (s *Service) GenerateInviteLink(ctx context.Context, userID, conversationID string) (string, error) {
	role, isMember, err := s.repo.MemberRoleOf(ctx, conversationID, userID)
	if err != nil {
		return "", err
	}
	if !isMember {
		return "", ErrNotAMember
	}
	if !isAdminOrOwner(role) {
		return "", ErrPermissionDenied
	}
	return s.repo.SetNewInviteCode(ctx, conversationID)
}

func (s *Service) RevokeInviteLink(ctx context.Context, userID, conversationID string) error {
	role, isMember, err := s.repo.MemberRoleOf(ctx, conversationID, userID)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrNotAMember
	}
	if !isAdminOrOwner(role) {
		return ErrPermissionDenied
	}
	return s.repo.RevokeInviteCode(ctx, conversationID)
}

func (s *Service) JoinViaInvite(ctx context.Context, userID, inviteCode string) (string, error) {
	g, err := s.repo.GetGroupByInviteCode(ctx, inviteCode)
	if err != nil {
		return "", err
	}
	if err := s.repo.AddMembers(ctx, g.ConversationID, []string{userID}); err != nil {
		return "", err
	}
	added := []Member{{UserID: userID, Role: RoleMember, JoinedAt: time.Now().UTC()}}
	s.broadcastToMembers(ctx, g.ConversationID, NewEnvelope(EventGroupMemberAdded, g.ConversationID, "", GroupMemberAddedPayload{Members: added, AddedBy: userID}))
	return g.ConversationID, nil
}

func (s *Service) SetMute(ctx context.Context, userID, conversationID string, until *time.Time) error {
	isMember, err := s.repo.IsMember(ctx, conversationID, userID)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrNotAMember
	}
	return s.repo.SetMuteUntil(ctx, conversationID, userID, until)
}

// broadcastToMembers fans an event out to every current member of a
// conversation — used for group metadata/membership changes, which
// (unlike message receipts) need to reach everyone, not just "the other
// side" of a 1:1. Errors are logged by Publish's caller context in a fuller
// implementation; here we best-effort continue so one bad publish doesn't
// stop the rest of the group from being notified.
func (s *Service) broadcastToMembers(ctx context.Context, conversationID string, env Envelope) {
	members, err := s.repo.ListMembers(ctx, conversationID)
	if err != nil {
		return
	}
	for _, m := range members {
		_ = s.hub.Publish(ctx, m.UserID, env)
	}
}
