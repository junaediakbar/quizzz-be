package repositories

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

type ProctoringRepository struct {
	db *sql.DB
}

func NewProctoringRepository(db *sql.DB) *ProctoringRepository {
	return &ProctoringRepository{db: db}
}

// ProctoringEvent is one integrity event recorded during an exam session.
type ProctoringEvent struct {
	ID        string          `json:"id"`
	SessionID string          `json:"session_id"`
	EventType string          `json:"event_type"`
	Detail    json.RawMessage `json:"detail,omitempty"`
	CreatedAt string          `json:"created_at"`
}

// ListEvents returns the integrity events of a session, oldest first.
func (r *ProctoringRepository) ListEvents(sessionID string) ([]ProctoringEvent, error) {
	q := `SELECT id, session_id, event_type, COALESCE(detail::text, ''), created_at
	      FROM exam_proctoring_events WHERE session_id = $1 ORDER BY created_at ASC`
	rows, err := r.db.Query(q, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list proctoring events: %w", err)
	}
	defer rows.Close()

	out := []ProctoringEvent{}
	for rows.Next() {
		var e ProctoringEvent
		var created sql.NullTime
		var detail string
		if err := rows.Scan(&e.ID, &e.SessionID, &e.EventType, &detail, &created); err != nil {
			return nil, err
		}
		if detail != "" {
			e.Detail = json.RawMessage(detail)
		}
		if created.Valid {
			e.CreatedAt = created.Time.UTC().Format("2006-01-02T15:04:05Z")
		}
		out = append(out, e)
	}
	return out, nil
}

func (r *ProctoringRepository) LogEvent(sessionID, eventType string, detail json.RawMessage) error {
	id := uuid.New().String()
	var detailVal interface{}
	if len(detail) > 0 {
		detailVal = string(detail)
	}
	q := `INSERT INTO exam_proctoring_events (id, session_id, event_type, detail) VALUES ($1, $2, $3, $4)`
	_, err := r.db.Exec(q, id, sessionID, eventType, detailVal)
	if err != nil {
		return fmt.Errorf("log proctoring event: %w", err)
	}
	return nil
}
