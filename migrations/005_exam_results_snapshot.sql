/* Snapshot judul & kelas ujian pada hasil (nama tetap sesuai saat ujian diselesaikan) */
ALTER TABLE exam_results ADD COLUMN IF NOT EXISTS exam_title VARCHAR(255);
ALTER TABLE exam_results ADD COLUMN IF NOT EXISTS exam_grade VARCHAR(50);

UPDATE exam_results er
SET
  exam_title = COALESCE(NULLIF(er.exam_title, ''), e.title),
  exam_grade = COALESCE(NULLIF(er.exam_grade, ''), e.grade)
FROM exams e
WHERE er.exam_id = e.id;
