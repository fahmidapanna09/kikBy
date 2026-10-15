package chat

import (
	"encoding/json"
	"strings"
	"testing"
)

// Guards a bug from Phase 2: Conversation had no json tags, so the API
// returned "ID"/"Type" and the Android client (which reads "id"/"type")
// couldn't parse the response to "start a conversation".
func TestConversation_JSONUsesSnakeCaseKeys(t *testing.T) {
	b, err := json.Marshal(Conversation{ID: "c1", Type: "direct"})
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if !strings.Contains(out, `"id":"c1"`) || !strings.Contains(out, `"type":"direct"`) {
		t.Fatalf("expected lowercase id/type keys, got %s", out)
	}
}

func TestConversationSummary_GroupNameOnlyWhenSet(t *testing.T) {
	direct, _ := json.Marshal(ConversationSummary{ID: "c1", Type: "direct", PeerUserID: "u2"})
	if strings.Contains(string(direct), `"name"`) {
		t.Fatalf("direct conversations must not carry a name, got %s", direct)
	}
	name := "Weekend Trip"
	group, _ := json.Marshal(ConversationSummary{ID: "c2", Type: "group", Name: &name})
	if !strings.Contains(string(group), `"name":"Weekend Trip"`) {
		t.Fatalf("group conversation should carry its name, got %s", group)
	}
	if strings.Contains(string(group), `"peer_user_id"`) {
		t.Fatalf("group conversations must not carry a peer_user_id, got %s", group)
	}
}
