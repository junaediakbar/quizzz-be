-- 009: tipe soal baru "step-by-step" (soal matematika bertahap).
-- Murid menjawab tiap langkah; penilaian parsial per langkah.

ALTER TABLE questions DROP CONSTRAINT IF EXISTS questions_type_check;

ALTER TABLE questions
    ADD CONSTRAINT questions_type_check
    CHECK (type IN ('multiple-choice', 'true-false', 'short-answer', 'essay', 'matching', 'fill-blank', 'step-by-step'));
