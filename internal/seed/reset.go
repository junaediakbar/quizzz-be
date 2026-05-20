package seed

import (
	"database/sql"
	"fmt"
)

// TruncateAll menghapus semua baris tabel aplikasi (urutan aman dengan CASCADE).
// Dipakai sebelum RunE2E agar dashboard murid / guru bersih dari data lama atau migrasi lawas.
func TruncateAll(db *sql.DB) error {
	_, err := db.Exec(`
		TRUNCATE TABLE
			exam_proctoring_events,
			answer_reviews,
			exam_results,
			exam_sessions,
			exam_questions,
			exams,
			question_bank_questions,
			questions,
			question_banks,
			activity_log,
			categories,
			users
		RESTART IDENTITY CASCADE
	`)
	if err != nil {
		return fmt.Errorf("truncate all: %w", err)
	}
	return nil
}
