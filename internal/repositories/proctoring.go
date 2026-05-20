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
