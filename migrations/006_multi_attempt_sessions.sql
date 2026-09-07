-- Allow multiple exam attempts per student, honoring exams.config.max_attempts.
ALTER TABLE exam_sessions ADD COLUMN IF NOT EXISTS attempt_number INTEGER NOT NULL DEFAULT 1;
ALTER TABLE exam_sessions DROP CONSTRAINT IF EXISTS exam_sessions_exam_id_student_id_key;
ALTER TABLE exam_sessions ADD CONSTRAINT exam_sessions_exam_student_attempt_key UNIQUE (exam_id, student_id, attempt_number);
