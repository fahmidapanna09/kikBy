package chat

import "context"

// presencePeersLimit caps how many people one connect/disconnect fans out to (huge groups should not turn every
// reconnect into thousands of Redis publishes).
const presencePeersLimit = 500

// PresencePeers returns the people who should hear that userID went online/offline, and whose status userID should be
// told when connecting: accepted friends plus everyone who shares an active conversation. Blocked pairs are excluded
// both ways so a block never leaks "is online".
func (r *Repository) PresencePeers(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT peer FROM (
			SELECT CASE WHEN f.requester_id = $1::uuid THEN f.addressee_id ELSE f.requester_id END AS peer
			FROM friendships f
			WHERE f.status = 'accepted' AND (f.requester_id = $1::uuid OR f.addressee_id = $1::uuid)
			UNION
			SELECT cm2.user_id AS peer
			FROM conversation_members cm1
			JOIN conversation_members cm2 ON cm2.conversation_id = cm1.conversation_id AND cm2.user_id <> cm1.user_id AND cm2.left_at IS NULL
			WHERE cm1.user_id = $1::uuid AND cm1.left_at IS NULL
		) p
		WHERE NOT EXISTS (
			SELECT 1 FROM blocked_users b
			WHERE (b.blocker_id = $1::uuid AND b.blocked_id = p.peer) OR (b.blocker_id = p.peer AND b.blocked_id = $1::uuid)
		)
		LIMIT $2`, userID, presencePeersLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// PresencePeers is the service-level wrapper used by the WebSocket handler.
func (s *Service) PresencePeers(ctx context.Context, userID string) ([]string, error) {
	return s.repo.PresencePeers(ctx, userID)
}
