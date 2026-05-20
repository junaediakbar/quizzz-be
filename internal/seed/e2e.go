package seed

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/junaediakbar/quizzz-backend/internal/services"
	"github.com/junaediakbar/quizzz-backend/models"
)

// Kredensial demo: lihat users.go (InsertDemoUsers, DemoUserSpecs).

// RunE2E replaces prior demo rows marked [Seed] / @seed.quizzz.dev and inserts a full chain:
// users → categories → questions → banks → exams → optional submitted session + result.
func RunE2E(db *sql.DB) error {
	hash, err := services.HashPassword(DemoPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := clearSeedScope(tx); err != nil {
		return fmt.Errorf("clear seed: %w", err)
	}

	// --- Users (sama dengan DemoUserSpecs + murid ketiga) ---
	var teacherID, studentID, student2ID, adminID string
	for _, spec := range DemoUserSpecs() {
		id := uuid.New().String()
		if _, execErr := tx.Exec(`
			INSERT INTO users (id, name, email, password_hash, role, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, id, spec.Name, spec.Email, hash, spec.Role); execErr != nil {
			return fmt.Errorf("insert user %s: %w", spec.Email, execErr)
		}
		switch spec.Email {
		case EmailTeacher:
			teacherID = id
		case EmailStudent:
			studentID = id
		case EmailStudent2:
			student2ID = id
		case EmailAdmin:
			adminID = id
		}
	}
	_, _ = student2ID, adminID

	// --- Categories ---
	var catIPA, catMath string
	err = tx.QueryRow(`
		INSERT INTO categories (name, slug, description, color, created_at, updated_at)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, "IPA Terpadu", "seed-ipa", "Kategori demo E2E", "#6366f1").Scan(&catIPA)
	if err != nil {
		return fmt.Errorf("category ipa: %w", err)
	}
	err = tx.QueryRow(`
		INSERT INTO categories (name, slug, description, color, created_at, updated_at)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, "Matematika", "seed-math", "Kategori demo E2E", "#0ea5e9").Scan(&catMath)
	if err != nil {
		return fmt.Errorf("category math: %w", err)
	}

	// --- Questions ---
	type qRow struct {
		id          string
		type_       string
		title       string
		content     string
		options     *string
		correct     string
		explanation *string
		difficulty  string
		points      int
		tags        *string
		categoryID  *string
	}
	questions := []qRow{
		{
			uuid.New().String(), "multiple-choice", "[Seed] Fotosintesis",
			"Produk utama fotosintesis pada tumbuhan adalah?",
			strPtr(`["Oksigen", "Karbon dioksida", "Nitrogen", "Hidrogen"]`),
			"Oksigen",
			strPtr("Oksigen dilepas saat fotosintesis."),
			"easy", 5, strPtr(`["ipa","biologi"]`), &catIPA,
		},
		{
			uuid.New().String(), "true-false", "[Seed] Rumus air",
			"Rumus kimia air adalah H₂O.",
			nil, "true",
			strPtr("Molekul air tersusun dari dua atom hidrogen dan satu oksigen."),
			"easy", 3, strPtr(`["ipa","kimia"]`), &catIPA,
		},
		{
			uuid.New().String(), "multiple-choice", "[Seed] Pembelahan sel",
			"Sel somatik membelah melalui proses?",
			strPtr(`["Meiosis", "Mitosis", "Fertilisasi", "Osmosis"]`),
			"Mitosis",
			strPtr("Mitosis menghasilkan dua sel anak identik."),
			"medium", 8, strPtr(`["ipa","biologi"]`), &catIPA,
		},
		{
			uuid.New().String(), "short-answer", "[Seed] Penjumlahan",
			"Berapakah 15 + 27?",
			nil, "42",
			strPtr("15 + 27 = 42"),
			"easy", 5, strPtr(`["matematika"]`), &catMath,
		},
		{
			uuid.New().String(), "multiple-choice", "[Seed] Persamaan kuadrat",
			"Diskriminan pada ax²+bx+c adalah?",
			strPtr(`["b²-4ac", "b²+4ac", "4ac-b²", "a²+b²"]`),
			"b²-4ac",
			strPtr("Diskriminan menentukan akar real."),
			"medium", 10, strPtr(`["matematika","aljabar"]`), &catMath,
		},
		// --- Semua tipe tambahan (essay, matching, fill-blank) ---
		{
			uuid.New().String(), "essay", "[Seed] Esai: energi terbarukan",
			"Jelaskan dua contoh energi terbarukan dan manfaatnya dalam kehidupan sehari-hari (minimal 3 kalimat).",
			nil,
			"Contoh kunci: tenaga surya dan angin; mengurangi ketergantungan bahan bakar fosil; ramah lingkungan (dinilai manual).",
			strPtr("Soal esai dinilai manual oleh guru."),
			"medium", 15, strPtr(`["ipa","energi","esai"]`), &catIPA,
		},
		{
			uuid.New().String(), "matching", "[Seed] Matching: provinsi & ibu kota",
			"Cocokkan: A=Aceh, B=Jawa Tengah, C=Papua dengan 1=Jayapura, 2=Banda Aceh, 3=Semarang. Tulis jawaban seperti A-2,B-3,C-1.",
			strPtr(`["A. Aceh","B. Jawa Tengah","C. Papua","1. Jayapura","2. Banda Aceh","3. Semarang"]`),
			"A-2,B-3,C-1",
			strPtr("Matching dinilai manual; opsi menampilkan pasangan untuk latihan."),
			"medium", 10, strPtr(`["ips","geografi"]`), &catIPA,
		},
		{
			uuid.New().String(), "fill-blank", "[Seed] Isian: planet",
			"Planet ketiga dari Matahari adalah _____.",
			nil,
			"Bumi",
			strPtr("Jawaban tidak case-sensitive."),
			"easy", 5, strPtr(`["ipa","astronomi"]`), &catIPA,
		},
	}

	qIDs := make([]string, len(questions))
	for i, q := range questions {
		_, err := tx.Exec(`
			INSERT INTO questions (id, type, title, content, options, correct_answer, explanation,
				difficulty, points, tags, category_id, created_by, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, q.id, q.type_, q.title, q.content, q.options, q.correct, q.explanation,
			q.difficulty, q.points, q.tags, q.categoryID, teacherID)
		if err != nil {
			return fmt.Errorf("insert question %s: %w", q.title, err)
		}
		qIDs[i] = q.id
	}

	// --- Question bank ---
	bankID := uuid.New().String()
	_, err = tx.Exec(`
		INSERT INTO question_banks (id, name, description, created_by, is_public, created_at, updated_at)
		VALUES ($1, $2, $3, $4, true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, bankID, "[Seed] Bank IPA", "Bank soal untuk demo", teacherID)
	if err != nil {
		return fmt.Errorf("bank: %w", err)
	}
	for ord, idx := range []int{0, 1, 2, 5, 6, 7} {
		_, err = tx.Exec(`
			INSERT INTO question_bank_questions (id, bank_id, question_id, order_index)
			VALUES ($1, $2, $3, $4)
		`, uuid.New().String(), bankID, qIDs[idx], ord)
		if err != nil {
			return fmt.Errorf("bank link: %w", err)
		}
	}

	// --- Exams (config uses snake_case JSON keys matching models.ExamConfig) ---
	cfgIPA := models.ExamConfig{
		Duration:         45,
		ShuffleQuestions: false,
		ShuffleOptions:   false,
		ShowResults:      "immediate",
		AllowReview:      true,
		MaxAttempts:      3,
		PassingScore:     60,
	}
	cfgIPAJSON, _ := json.Marshal(cfgIPA)

	cfgMath := models.ExamConfig{
		Duration:         20,
		ShuffleQuestions: false,
		ShuffleOptions:   true,
		ShowResults:      "after-review",
		AllowReview:      true,
		MaxAttempts:      2,
		PassingScore:     70,
	}
	cfgMathJSON, _ := json.Marshal(cfgMath)

	startPast := time.Now().Add(-48 * time.Hour)
	endFuture := time.Now().Add(30 * 24 * time.Hour)

	examIPA := uuid.New().String()
	examMath := uuid.New().String()
	examDraft := uuid.New().String()

	_, err = tx.Exec(`
		INSERT INTO exams (id, title, description, subject, grade, config, status, created_by, scheduled_start, scheduled_end, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'published', $7, $8, $9, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, examIPA, "[Seed] Ujian IPA Terpadu", strPtr("Demo lengkap: login murid → mulai ujian → kirim jawaban."),
		"IPA", "SMP", string(cfgIPAJSON), teacherID, startPast, endFuture)
	if err != nil {
		return fmt.Errorf("exam ipa: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO exams (id, title, description, subject, grade, config, status, created_by, scheduled_start, scheduled_end, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'active', $7, $8, $9, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, examMath, "[Seed] Latihan Matematika", strPtr("Status active + jadwal masih berlaku."),
		"Matematika", "SMP", string(cfgMathJSON), teacherID, startPast, endFuture)
	if err != nil {
		return fmt.Errorf("exam math: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO exams (id, title, description, subject, grade, config, status, created_by, scheduled_start, scheduled_end, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'draft', $7, NULL, NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, examDraft, "[Seed] Draf Ujian (opsional)", strPtr("Hanya terlihat guru/admin."),
		"IPA", "SMP", string(cfgIPAJSON), teacherID)
	if err != nil {
		return fmt.Errorf("exam draft: %w", err)
	}

	// Link exam IPA: questions 0-3
	for ord, idx := range []int{0, 1, 2, 3} {
		points := []int{5, 3, 8, 5}[ord]
		_, err = tx.Exec(`
			INSERT INTO exam_questions (id, exam_id, question_id, order_index, points)
			VALUES ($1, $2, $3, $4, $5)
		`, uuid.New().String(), examIPA, qIDs[idx], ord, points)
		if err != nil {
			return fmt.Errorf("exam ipa q: %w", err)
		}
	}
	// Link math exam: short-answer, MCQ, fill-blank
	for ord, idx := range []int{3, 4, 7} {
		points := []int{5, 10, 5}[ord]
		_, err = tx.Exec(`
			INSERT INTO exam_questions (id, exam_id, question_id, order_index, points)
			VALUES ($1, $2, $3, $4, $5)
		`, uuid.New().String(), examMath, qIDs[idx], ord, points)
		if err != nil {
			return fmt.Errorf("exam math q: %w", err)
		}
	}

	// Draft: semua tipe soal dalam satu ujian (demo guru)
	examAllTypes := uuid.New().String()
	_, err = tx.Exec(`
		INSERT INTO exams (id, title, description, subject, grade, config, status, created_by, scheduled_start, scheduled_end, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'draft', $7, NULL, NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, examAllTypes, "[Seed] Semua Tipe Soal", strPtr("Berisi MCQ, benar-salah, singkat, esai, matching, isian — untuk uji coba."),
		"Campuran", "Umum", string(cfgIPAJSON), teacherID)
	if err != nil {
		return fmt.Errorf("exam all types: %w", err)
	}
	for ord, idx := range []int{0, 1, 2, 3, 4, 5, 6, 7} {
		points := []int{5, 3, 8, 5, 10, 15, 10, 5}[ord]
		_, err = tx.Exec(`
			INSERT INTO exam_questions (id, exam_id, question_id, order_index, points)
			VALUES ($1, $2, $3, $4, $5)
		`, uuid.New().String(), examAllTypes, qIDs[idx], ord, points)
		if err != nil {
			return fmt.Errorf("exam all types q: %w", err)
		}
	}

	// --- One finished session (student1 on IPA exam) — answers keyed by question UUID ---
	answers := map[string]string{
		qIDs[0]: "Oksigen",
		qIDs[1]: "true",
		qIDs[2]: "Mitosis",
		qIDs[3]: "42",
	}
	ansJSON, _ := json.Marshal(answers)
	maxScore := 5 + 3 + 8 + 5 // 21
	score := 21
	submitted := time.Now().Add(-2 * time.Hour)
	sessionID := uuid.New().String()

	_, err = tx.Exec(`
		INSERT INTO exam_sessions (id, exam_id, student_id, answers, status, started_at, submitted_at, time_spent, score, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'submitted', $5, $6, 1200, $7, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, sessionID, examIPA, studentID, string(ansJSON), submitted.Add(-30*time.Minute), submitted, score)
	if err != nil {
		return fmt.Errorf("session: %w", err)
	}

	// Answer reviews (mirror grading)
	reviews := []struct {
		qid       string
		stu       string
		corr      string
		ok        bool
		pts, maxp int
	}{
		{qIDs[0], "Oksigen", "Oksigen", true, 5, 5},
		{qIDs[1], "true", "true", true, 3, 3},
		{qIDs[2], "Mitosis", "Mitosis", true, 8, 8},
		{qIDs[3], "42", "42", true, 5, 5},
	}
	for _, r := range reviews {
		_, err = tx.Exec(`
			INSERT INTO answer_reviews (session_id, question_id, student_answer, correct_answer, is_correct, points, max_points)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, sessionID, r.qid, r.stu, r.corr, r.ok, r.pts, r.maxp)
		if err != nil {
			return fmt.Errorf("review: %w", err)
		}
	}

	resultID := uuid.New().String()
	_, err = tx.Exec(`
		INSERT INTO exam_results (id, session_id, exam_id, student_id, student_name, exam_title, exam_grade, score, max_score, passed, time_spent, submitted_at, graded_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, true, 1200, $10, $11)
	`, resultID, sessionID, examIPA, studentID, "Budi Murid", "[Seed] Ujian IPA Terpadu", "7", score, maxScore, submitted, submitted)
	if err != nil {
		return fmt.Errorf("result: %w", err)
	}

	// Activity
	_, err = tx.Exec(`
		INSERT INTO activity_log (user_id, type, title, description, metadata, created_at)
		VALUES ($1, 'exam-submitted', 'Ujian diserahkan', $2, $3, CURRENT_TIMESTAMP)
	`, studentID, "Murid menyelesaikan [Seed] Ujian IPA Terpadu",
		strPtr(`{"exam_id":"`+examIPA+`","score":21}`))
	if err != nil {
		return fmt.Errorf("activity: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("=== E2E seed selesai ===")
	fmt.Printf("Password untuk semua akun demo: %s\n", DemoPassword)
	fmt.Println("  Guru:     ", EmailTeacher)
	fmt.Println("  Murid 1:  ", EmailStudent, " (sudah punya riwayat ujian IPA)")
	fmt.Println("  Murid 2:  ", EmailStudent2, " (belum ujian — bisa mulai dari dashboard)")
	fmt.Println("  Murid 3:  ", EmailStudent3, " (belum ujian)")
	fmt.Println("  Admin:    ", EmailAdmin)
	fmt.Println()
	fmt.Println("Ujian murid (published/active):")
	fmt.Println("  ", examIPA, " — [Seed] Ujian IPA Terpadu")
	fmt.Println("  ", examMath, " — [Seed] Latihan Matematika (singkat + kuadrat + isian)")
	fmt.Println("Draft (semua tipe soal):")
	fmt.Println("  ", examAllTypes, " — [Seed] Semua Tipe Soal")
	fmt.Println()
	return nil
}

func clearSeedScope(tx *sql.Tx) error {
	steps := []string{
		`DELETE FROM exam_proctoring_events WHERE session_id IN (
			SELECT es.id FROM exam_sessions es
			INNER JOIN exams e ON e.id = es.exam_id WHERE e.title LIKE '[Seed]%'
		)`,
		`DELETE FROM answer_reviews WHERE session_id IN (
			SELECT es.id FROM exam_sessions es
			INNER JOIN exams e ON e.id = es.exam_id WHERE e.title LIKE '[Seed]%'
		)`,
		`DELETE FROM exam_results WHERE exam_id IN (SELECT id FROM exams WHERE title LIKE '[Seed]%')`,
		`DELETE FROM exam_sessions WHERE exam_id IN (SELECT id FROM exams WHERE title LIKE '[Seed]%')`,
		`DELETE FROM exam_questions WHERE exam_id IN (SELECT id FROM exams WHERE title LIKE '[Seed]%')`,
		`DELETE FROM exams WHERE title LIKE '[Seed]%'`,
		`DELETE FROM question_bank_questions WHERE bank_id IN (SELECT id FROM question_banks WHERE name LIKE '[Seed]%')`,
		`DELETE FROM question_banks WHERE name LIKE '[Seed]%'`,
		`DELETE FROM questions WHERE title LIKE '[Seed]%'`,
		`DELETE FROM activity_log WHERE user_id IN (SELECT id FROM users WHERE email LIKE '%@seed.quizzz.dev')`,
		`DELETE FROM users WHERE email LIKE '%@seed.quizzz.dev'`,
	}
	for _, q := range steps {
		if _, err := tx.Exec(q); err != nil {
			return fmt.Errorf("cleanup: %w", err)
		}
	}
	return nil
}

func strPtr(s string) *string { return &s }
