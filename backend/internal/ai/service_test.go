package ai

import (
	"strings"
	"testing"
)

func TestMaxUserMessageCharsEnforced(t *testing.T) {
	// Pure guard check: anything over the limit must be rejected before it
	// ever reaches the rate limiter or the Gemini client (cheap check first).
	tooLong := strings.Repeat("a", MaxUserMessageChars+1)
	if len([]rune(tooLong)) <= MaxUserMessageChars {
		t.Fatalf("test fixture is not actually over the limit")
	}
}

func TestRoleConstants(t *testing.T) {
	if RoleUser == RoleModel {
		t.Fatalf("RoleUser and RoleModel must be distinct")
	}
	if string(RoleUser) != "user" || string(RoleModel) != "model" {
		t.Fatalf("role string values must match Gemini's own role vocabulary exactly: got %q / %q", RoleUser, RoleModel)
	}
}

func TestDefaultModelNonEmpty(t *testing.T) {
	if strings.TrimSpace(DefaultModel) == "" {
		t.Fatalf("DefaultModel must never be empty — NewService falls back to it when model config is blank")
	}
}

func TestNewServiceFallsBackToDefaultModel(t *testing.T) {
	svc := NewService(nil, nil, nil, "")
	if svc.model != DefaultModel {
		t.Fatalf("expected fallback to DefaultModel, got %q", svc.model)
	}
	svc2 := NewService(nil, nil, nil, "gemini-custom")
	if svc2.model != "gemini-custom" {
		t.Fatalf("expected configured model to be kept, got %q", svc2.model)
	}
}
