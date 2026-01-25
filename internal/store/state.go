package store

import "database/sql"

type StateCurrentRow struct {
	Workspace   string
	StateJSON   string
	StateTokens int
	UpdatedAt   string
}

type StateHistoryRow struct {
	StateJSON string
	Reason    string
	CreatedAt string
}

func (s *Store) ListStateCurrent(repoID string) ([]StateCurrentRow, error) {
	rows, err := s.db.Query(`
		SELECT workspace, state_json, state_tokens, updated_at
		FROM state_current
		WHERE repo_id = ?
	`, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []StateCurrentRow
	for rows.Next() {
		var row StateCurrentRow
		var tokens sql.NullInt64
		if err := rows.Scan(&row.Workspace, &row.StateJSON, &tokens, &row.UpdatedAt); err != nil {
			return nil, err
		}
		if tokens.Valid {
			row.StateTokens = int(tokens.Int64)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) GetLatestStateHistory(repoID, workspace string) (StateHistoryRow, error) {
	row := s.db.QueryRow(`
		SELECT state_json, reason, created_at
		FROM state_history
		WHERE repo_id = ? AND workspace = ?
		ORDER BY created_at DESC
		LIMIT 1
	`, repoID, workspace)

	var stateJSON string
	var reason sql.NullString
	var createdAt string
	if err := row.Scan(&stateJSON, &reason, &createdAt); err != nil {
		return StateHistoryRow{}, err
	}
	return StateHistoryRow{
		StateJSON: stateJSON,
		Reason:    reason.String,
		CreatedAt: createdAt,
	}, nil
}
