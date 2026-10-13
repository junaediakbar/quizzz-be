-- 007: Bagian/part soal dalam satu ujian (contoh "Bagian A: Pilihan Ganda", "Bagian B: Uraian").
-- Soal tetap disimpan di exam_questions. part_id hanya mengelompokkan soal ke bagiannya.

CREATE TABLE IF NOT EXISTS exam_parts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    exam_id UUID NOT NULL REFERENCES exams(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    order_index INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_exam_parts_exam_id ON exam_parts(exam_id);

ALTER TABLE exam_questions
    ADD COLUMN IF NOT EXISTS part_id UUID REFERENCES exam_parts(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_exam_questions_part_id ON exam_questions(part_id);
