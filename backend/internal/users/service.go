package users

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/yourorg/chatplatform/internal/platform/ratelimit"
)

var (
	ErrInvalidUsername  = errors.New("users: username must be 3-20 characters, start with a letter, and contain only lowercase letters, numbers, or underscores")
	ErrRateLimited      = errors.New("users: rate limited")
	ErrCannotReportSelf = errors.New("users: cannot report yourself")
	ErrNoAvatar         = errors.New("users: no profile picture")
	ErrBadAvatar        = errors.New("users: profile picture must be an uploaded JPEG, PNG or WebP image of yours")
)

// usernamePattern intentionally requires lowercase — usernames are matched
// case-insensitively at the DB layer (see migration 0001's unique index on
// lower(username)), so normalizing to lowercase here avoids a confusing gap
// between "what's stored" and "what's displayed".
var usernamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{2,19}$`)

// AvatarMedia is satisfied by *media.Service (wired in main.go) without importing the media package.
type AvatarMedia interface {
	VerifyOwnedUploaded(ctx context.Context, userID, mediaID string) (kind string, mimeType string, err error)
	PresignAvatar(ctx context.Context, mediaID string) (string, error)
}

type Service struct {
	repo    *Repository
	limiter *ratelimit.Limiter
	avatar  AvatarMedia // nil until SetAvatarMedia: profile pictures are then refused
}

// SetAvatarMedia enables profile pictures (the media service is built after this one, hence a setter).
func (s *Service) SetAvatarMedia(m AvatarMedia) { s.avatar = m }

func NewService(repo *Repository, limiter *ratelimit.Limiter) *Service {
	return &Service{repo: repo, limiter: limiter}
}

func (s *Service) GetMe(ctx context.Context, userID string) (MeResponse, error) {
	return s.repo.GetMe(ctx, userID)
}

func (s *Service) UpdateProfile(ctx context.Context, userID string, req UpdateProfileRequest) (MeResponse, error) {
	if req.LastSeenVisibility != nil {
		v := *req.LastSeenVisibility
		if v != "everyone" && v != "contacts" && v != "nobody" {
			return MeResponse{}, errors.New("users: last_seen_visibility must be everyone, contacts, or nobody")
		}
	}
	if req.ContactPhone != nil && *req.ContactPhone != "" && !isValidE164(*req.ContactPhone) {
		return MeResponse{}, errors.New("users: contact_phone must be in E.164 format")
	}
	if req.AvatarMediaID != nil && *req.AvatarMediaID != "" {
		// Only the caller's own, finished image upload may become the profile picture (the public avatar link serves it to everyone).
		if s.avatar == nil {
			return MeResponse{}, ErrBadAvatar
		}
		kind, mime, err := s.avatar.VerifyOwnedUploaded(ctx, userID, *req.AvatarMediaID)
		if err != nil || kind != "image" || (mime != "image/jpeg" && mime != "image/png" && mime != "image/webp") {
			return MeResponse{}, ErrBadAvatar
		}
	}
	if err := s.repo.UpdateProfile(ctx, userID, req); err != nil {
		return MeResponse{}, err
	}
	return s.repo.GetMe(ctx, userID)
}

// AvatarURL returns a short-lived link to the user's profile picture, or ErrNoAvatar.
func (s *Service) AvatarURL(ctx context.Context, userID string) (string, error) {
	if s.avatar == nil {
		return "", ErrNoAvatar
	}
	mediaID, err := s.repo.AvatarMediaID(ctx, userID)
	if err != nil {
		return "", err
	}
	if mediaID == "" {
		return "", ErrNoAvatar
	}
	return s.avatar.PresignAvatar(ctx, mediaID)
}

func (s *Service) UpdateUsername(ctx context.Context, userID, newUsername string) (MeResponse, error) {
	if !usernamePattern.MatchString(newUsername) {
		return MeResponse{}, ErrInvalidUsername
	}
	if err := s.repo.UpdateUsername(ctx, userID, newUsername); err != nil {
		return MeResponse{}, err
	}
	return s.repo.GetMe(ctx, userID)
}

func (s *Service) GetPublicProfile(ctx context.Context, userID string) (PublicProfile, error) {
	return s.repo.GetPublicProfile(ctx, userID)
}

func (s *Service) SearchByUsername(ctx context.Context, requestingUserID, prefix string) (SearchResponse, error) {
	ok, err := s.limiter.Allow(ctx, "search:username:"+requestingUserID, 30, time.Minute)
	if err != nil {
		return SearchResponse{}, err
	}
	if !ok {
		return SearchResponse{}, ErrRateLimited
	}
	users, err := s.repo.SearchByUsername(ctx, prefix, 20)
	if err != nil {
		return SearchResponse{}, err
	}
	return SearchResponse{Users: users}, nil
}

// SearchByPhone is far more tightly rate-limited than username search
// (5/hour vs 30/minute) specifically because a phone number is personal
// information about a third party (whoever's in the requester's contact
// list), not a chosen-to-be-discoverable handle like a username — see spec
// section 3's "anti-abuse/rate limiting for phone-number search".
func (s *Service) SearchByPhone(ctx context.Context, requestingUserID, phoneNumber string) (SearchResponse, error) {
	ok, err := s.limiter.Allow(ctx, "search:phone:"+requestingUserID, 5, time.Hour)
	if err != nil {
		return SearchResponse{}, err
	}
	if !ok {
		return SearchResponse{}, ErrRateLimited
	}
	profile, err := s.repo.GetPublicProfileByPhone(ctx, phoneNumber)
	if errors.Is(err, ErrNotFound) {
		return SearchResponse{Users: []PublicProfile{}}, nil // not registered — empty result, not an error
	}
	if err != nil {
		return SearchResponse{}, err
	}
	return SearchResponse{Users: []PublicProfile{profile}}, nil
}

func (s *Service) ReportUser(ctx context.Context, reporterID, reportedID, reason string, contextMessageID *string) error {
	if reporterID == reportedID {
		return ErrCannotReportSelf
	}
	if reason == "" {
		return errors.New("users: reason is required")
	}
	return s.repo.ReportUser(ctx, reporterID, reportedID, reason, contextMessageID)
}
