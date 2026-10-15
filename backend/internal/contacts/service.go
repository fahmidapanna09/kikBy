package contacts

import (
	"context"
	"errors"
	"regexp"
)

var (
	ErrInvalidPhoneNumber = errors.New("contacts: phone number must be in E.164 format")
	ErrCannotBlockSelf    = errors.New("contacts: cannot block yourself")
	ErrTooManyHashes      = errors.New("contacts: too many phone hashes in one sync request")
)

var e164Pattern = regexp.MustCompile(`^\+[1-9]\d{6,14}$`)

// maxSyncBatch caps how many hashes a single /contacts/sync call accepts —
// a real device contact list can be large, so the Android client should
// batch (e.g. 500 at a time) rather than sending its entire address book in
// one request.
const maxSyncBatch = 1000

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) AddContact(ctx context.Context, ownerID string, req AddContactRequest) (Contact, error) {
	if !e164Pattern.MatchString(req.PhoneNumber) {
		return Contact{}, ErrInvalidPhoneNumber
	}
	return s.repo.AddContact(ctx, ownerID, req.PhoneNumber, req.Nickname, "manual")
}

func (s *Service) ListContacts(ctx context.Context, ownerID string) (ListContactsResponse, error) {
	list, err := s.repo.ListContacts(ctx, ownerID)
	if err != nil {
		return ListContactsResponse{}, err
	}
	return ListContactsResponse{Contacts: list}, nil
}

func (s *Service) UpdateContact(ctx context.Context, ownerID, contactID string, req UpdateContactRequest) error {
	if req.Nickname == nil {
		return nil // nothing to update
	}
	return s.repo.UpdateNickname(ctx, ownerID, contactID, *req.Nickname)
}

func (s *Service) DeleteContact(ctx context.Context, ownerID, contactID string) error {
	return s.repo.DeleteContact(ctx, ownerID, contactID)
}

// Sync matches client-computed phone-number hashes against registered
// accounts. It does NOT persist a contacts row for every match — that's a
// deliberate choice: syncing tells the client "these of your contacts are
// on the platform", and the client (or a follow-up explicit AddContact
// call) decides whether to actually save each one, same as WhatsApp
// distinguishes "contacts on WhatsApp" from your saved contact list.
func (s *Service) Sync(ctx context.Context, req SyncRequest) (SyncResponse, error) {
	if len(req.PhoneHashes) > maxSyncBatch {
		return SyncResponse{}, ErrTooManyHashes
	}
	matches, err := s.repo.MatchPhoneHashes(ctx, req.PhoneHashes)
	if err != nil {
		return SyncResponse{}, err
	}
	return SyncResponse{Matches: matches}, nil
}

func (s *Service) BlockUser(ctx context.Context, blockerID, blockedID string) error {
	if blockerID == blockedID {
		return ErrCannotBlockSelf
	}
	return s.repo.BlockUser(ctx, blockerID, blockedID)
}

func (s *Service) UnblockUser(ctx context.Context, blockerID, blockedID string) error {
	return s.repo.UnblockUser(ctx, blockerID, blockedID)
}

func (s *Service) ListBlocked(ctx context.Context, blockerID string) (ListBlockedResponse, error) {
	list, err := s.repo.ListBlocked(ctx, blockerID)
	if err != nil {
		return ListBlockedResponse{}, err
	}
	return ListBlockedResponse{Blocked: list}, nil
}
