package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

// Deprecated legacy seed: drops ALL tables and recreates schema from embedded SQL.
// Prefer migrations (migrations/*.sql) plus `go run ./cmd/seed` for safe E2E demo data.

func main() {
	_ = godotenv.Load()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	// Connect to database
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	// Test connection
	if err := db.Ping(); err != nil {
		log.Fatal("Failed to ping database:", err)
	}

	fmt.Println("Connected to Neon PostgreSQL!")

	// First, run the schema
	if err := runSchema(db); err != nil {
		log.Fatal("Failed to run schema:", err)
	}
	fmt.Println("Schema created successfully!")

	// Seed data
	if err := seedData(db); err != nil {
		log.Fatal("Failed to seed data:", err)
	}
	fmt.Println("Data seeded successfully!")
}

func runSchema(db *sql.DB) error {
	schema := `
	-- Enable UUID extension
	CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

	-- Drop existing tables
	DROP TABLE IF EXISTS activity_log CASCADE;
	DROP TABLE IF EXISTS answer_reviews CASCADE;
	DROP TABLE IF EXISTS exam_results CASCADE;
	DROP TABLE IF EXISTS exam_sessions CASCADE;
	DROP TABLE IF EXISTS exam_questions CASCADE;
	DROP TABLE IF EXISTS exams CASCADE;
	DROP TABLE IF EXISTS question_bank_questions CASCADE;
	DROP TABLE IF EXISTS question_banks CASCADE;
	DROP TABLE IF EXISTS questions CASCADE;
	DROP TABLE IF EXISTS categories CASCADE;
	DROP TABLE IF EXISTS users CASCADE;

	-- Users table
	CREATE TABLE users (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		name VARCHAR(255) NOT NULL,
		email VARCHAR(255) UNIQUE NOT NULL,
		password_hash VARCHAR(255) NOT NULL,
		role VARCHAR(20) NOT NULL CHECK (role IN ('teacher', 'student', 'admin')),
		avatar TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Categories table
	CREATE TABLE categories (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		name VARCHAR(255) NOT NULL,
		slug VARCHAR(255) UNIQUE NOT NULL,
		description TEXT,
		parent_id UUID REFERENCES categories(id) ON DELETE SET NULL,
		color VARCHAR(7),
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Question Banks table
	CREATE TABLE question_banks (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		name VARCHAR(255) NOT NULL,
		description TEXT,
		created_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		is_public BOOLEAN DEFAULT false,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Questions table
	CREATE TABLE questions (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		type VARCHAR(50) NOT NULL CHECK (type IN ('multiple-choice', 'true-false', 'short-answer', 'essay', 'matching', 'fill-blank')),
		title VARCHAR(500) NOT NULL,
		content TEXT NOT NULL,
		options JSONB,
		correct_answer TEXT NOT NULL,
		explanation TEXT,
		difficulty VARCHAR(20) CHECK (difficulty IN ('easy', 'medium', 'hard')),
		points INTEGER DEFAULT 5,
		tags JSONB,
		category_id UUID REFERENCES categories(id) ON DELETE SET NULL,
		created_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Question Banks Questions junction table
	CREATE TABLE question_bank_questions (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		bank_id UUID NOT NULL REFERENCES question_banks(id) ON DELETE CASCADE,
		question_id UUID NOT NULL REFERENCES questions(id) ON DELETE CASCADE,
		order_index INTEGER DEFAULT 0,
		UNIQUE(bank_id, question_id)
	);

	-- Exams table
	CREATE TABLE exams (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		title VARCHAR(500) NOT NULL,
		description TEXT,
		subject VARCHAR(255) NOT NULL,
		grade VARCHAR(50),
		config JSONB NOT NULL,
		status VARCHAR(20) DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'active', 'completed', 'archived')),
		created_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		scheduled_start TIMESTAMP,
		scheduled_end TIMESTAMP,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Exam Questions junction table
	CREATE TABLE exam_questions (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		exam_id UUID NOT NULL REFERENCES exams(id) ON DELETE CASCADE,
		question_id UUID NOT NULL REFERENCES questions(id) ON DELETE CASCADE,
		order_index INTEGER DEFAULT 0,
		points INTEGER DEFAULT 5,
		UNIQUE(exam_id, question_id)
	);

	-- Exam Sessions table
	CREATE TABLE exam_sessions (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		exam_id UUID NOT NULL REFERENCES exams(id) ON DELETE CASCADE,
		student_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		answers JSONB,
		status VARCHAR(20) DEFAULT 'not-started' CHECK (status IN ('not-started', 'in-progress', 'submitted', 'graded')),
		started_at TIMESTAMP,
		submitted_at TIMESTAMP,
		time_spent INTEGER DEFAULT 0,
		score INTEGER,
		graded_by UUID REFERENCES users(id) ON DELETE SET NULL,
		graded_at TIMESTAMP,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(exam_id, student_id)
	);

	-- Answer Reviews table
	CREATE TABLE answer_reviews (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		session_id UUID NOT NULL REFERENCES exam_sessions(id) ON DELETE CASCADE,
		question_id UUID NOT NULL REFERENCES questions(id) ON DELETE CASCADE,
		student_answer TEXT,
		correct_answer TEXT,
		is_correct BOOLEAN,
		points INTEGER DEFAULT 0,
		max_points INTEGER DEFAULT 0,
		feedback TEXT,
		UNIQUE(session_id, question_id)
	);

	-- Exam Results table
	CREATE TABLE exam_results (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		session_id UUID NOT NULL REFERENCES exam_sessions(id) ON DELETE CASCADE,
		exam_id UUID NOT NULL REFERENCES exams(id) ON DELETE CASCADE,
		student_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		student_name VARCHAR(255) NOT NULL,
		score INTEGER NOT NULL,
		max_score INTEGER NOT NULL,
		passed BOOLEAN NOT NULL,
		time_spent INTEGER NOT NULL,
		submitted_at TIMESTAMP NOT NULL,
		graded_at TIMESTAMP
	);

	-- Activity Log table
	CREATE TABLE activity_log (
		id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
		user_id UUID REFERENCES users(id) ON DELETE SET NULL,
		type VARCHAR(50) NOT NULL,
		title VARCHAR(255) NOT NULL,
		description TEXT,
		metadata JSONB,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- Indexes
	CREATE INDEX idx_users_email ON users(email);
	CREATE INDEX idx_users_role ON users(role);
	CREATE INDEX idx_questions_type ON questions(type);
	CREATE INDEX idx_questions_difficulty ON questions(difficulty);
	CREATE INDEX idx_exams_status ON exams(status);
	CREATE INDEX idx_exam_sessions_exam_id ON exam_sessions(exam_id);
	CREATE INDEX idx_exam_sessions_student_id ON exam_sessions(student_id);
	CREATE INDEX idx_exam_results_exam_id ON exam_results(exam_id);
	CREATE INDEX idx_exam_results_student_id ON exam_results(student_id);
	`

	_, err := db.Exec(schema)
	return err
}

// Helper functions
func strPtr(s string) *string {
	return &s
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func intPtr(i int) *int {
	return &i
}

func seedData(db *sql.DB) error {
	passwordHash := "$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewY5GyYzpLaEmc1a"

	// Store IDs after insertion for later use
	var teacherID1, teacherID2, studentID1, studentID2, studentID3 string
	var catMath, catBio, catChem, catPhys string

	// ===== USERS =====
	fmt.Println("Seeding Users...")

	userIDs := make(map[string]string)

	users := []struct {
		name   string
		email  string
		role   string
		avatar *string
	}{
		{"Dr. Sarah Johnson", "sarah.johnson@school.edu", "teacher", strPtr("/avatars/teacher-1.png")},
		{"Prof. Michael Chen", "michael.chen@school.edu", "teacher", strPtr("/avatars/teacher-2.png")},
		{"Ahmad Wijaya", "ahmad.wijaya@school.edu", "student", nil},
		{"Siti Rahayu", "siti.rahayu@school.edu", "student", nil},
		{"Budi Santoso", "budi.santoso@school.edu", "student", nil},
		{"Dewi Lestari", "dewi.lestari@school.edu", "student", nil},
		{"Rizky Pratama", "rizky.pratama@school.edu", "student", nil},
		{"Maya Putri", "maya.putri@school.edu", "student", nil},
		{"System Admin", "admin@quizapp.edu", "admin", nil},
	}

	for i, u := range users {
		var id string
		err := db.QueryRow(`
			INSERT INTO users (name, email, password_hash, role, avatar, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name
			RETURNING id
		`, u.name, u.email, passwordHash, u.role, u.avatar).Scan(&id)
		if err != nil {
			return fmt.Errorf("failed to insert user %s: %w", u.name, err)
		}
		userIDs[u.email] = id

		// Store specific IDs for later use
		if i == 0 { teacherID1 = id }
		if i == 1 { teacherID2 = id }
		if i == 2 { studentID1 = id }
		if i == 3 { studentID2 = id }
		if i == 4 { studentID3 = id }
		if i == 5 { _ = id } // studentID4 - unused
		if i == 6 { _ = id } // studentID5 - unused
		if i == 7 { _ = id } // studentID6 - unused
	}
	fmt.Println("✓ Users seeded")

	// ===== CATEGORIES =====
	fmt.Println("Seeding Categories...")

	categories := []struct {
		name        string
		slug        string
		description *string
		color       *string
	}{
		{"Mathematics", "mathematics", strPtr("Math questions and problems"), strPtr("#3b82f6")},
		{"Biology", "biology", strPtr("Biology and life sciences"), strPtr("#22c55e")},
		{"Chemistry", "chemistry", strPtr("Chemistry questions"), strPtr("#f59e0b")},
		{"Physics", "physics", strPtr("Physics and physical sciences"), strPtr("#8b5cf6")},
	}

	for i, c := range categories {
		var id string
		err := db.QueryRow(`
			INSERT INTO categories (name, slug, description, color, created_at, updated_at)
			VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
			RETURNING id
		`, c.name, c.slug, c.description, c.color).Scan(&id)
		if err != nil {
			return fmt.Errorf("failed to insert category %s: %w", c.name, err)
		}

		if i == 0 { catMath = id }
		if i == 1 { catBio = id }
		if i == 2 { catChem = id }
		if i == 3 { catPhys = id }
	}
	fmt.Println("✓ Categories seeded")

	// ===== QUESTIONS =====
	fmt.Println("Seeding Questions...")

	questions := []struct {
		type_       string
		title       string
		content     string
		options     *string
		correctAns  string
		explanation *string
		difficulty  string
		points      int
		tags        *string
		categoryID  *string
		createdBy   string
	}{
		{
			"multiple-choice", "Photosynthesis Process",
			"What is the primary product of photosynthesis?",
			strPtr(`["Oxygen", "Carbon Dioxide", "Nitrogen", "Hydrogen"]`),
			"Oxygen",
			strPtr("During photosynthesis, plants use sunlight, water, and carbon dioxide to produce glucose and oxygen."),
			"easy", 5, strPtr(`["biology", "plants", "photosynthesis"]`), &catBio, teacherID1,
		},
		{
			"multiple-choice", "Newton's Laws",
			"Which of Newton's laws states that for every action, there is an equal and opposite reaction?",
			strPtr(`["First Law", "Second Law", "Third Law", "Fourth Law"]`),
			"Third Law",
			strPtr("Newton's Third Law of Motion states that for every action, there is an equal and opposite reaction."),
			"medium", 10, strPtr(`["physics", "mechanics", "newton"]`), &catPhys, teacherID1,
		},
		{
			"true-false", "Chemical Bonding",
			"Covalent bonds involve the sharing of electron pairs between atoms.",
			nil, "true",
			strPtr("Covalent bonds indeed involve the sharing of electron pairs between atoms."),
			"easy", 3, strPtr(`["chemistry", "bonding"]`), &catChem, teacherID1,
		},
		{
			"multiple-choice", "Cell Division",
			"What is the process by which a cell divides into two identical daughter cells?",
			strPtr(`["Meiosis", "Mitosis", "Fertilization", "Osmosis"]`),
			"Mitosis",
			strPtr("Mitosis is the process of cell division that results in two genetically identical daughter cells."),
			"medium", 8, strPtr(`["biology", "cells", "division"]`), &catBio, teacherID1,
		},
		{
			"short-answer", "Mathematical Expression",
			"Simplify: (2x + 3y) + (4x - 2y)",
			nil, "6x + y",
			strPtr("Combine like terms: (2x + 4x) + (3y - 2y) = 6x + y"),
			"medium", 10, strPtr(`["math", "algebra"]`), &catMath, teacherID1,
		},
		{
			"multiple-choice", "Atomic Structure",
			"What is the smallest particle of an element that retains its chemical properties?",
			strPtr(`["Proton", "Neutron", "Electron", "Atom"]`),
			"Atom",
			strPtr("An atom is the smallest unit of an element that maintains the chemical properties of that element."),
			"easy", 5, strPtr(`["chemistry", "atoms"]`), &catChem, teacherID2,
		},
		{
			"multiple-choice", "Quadratic Equations",
			"What is the quadratic formula?",
			strPtr(`["x = (-b ± √(b²-4ac)) / 2a", "x = -b/a", "x = b²-4ac", "x = ax²+bx+c"]`),
			"x = (-b ± √(b²-4ac)) / 2a",
			strPtr("The quadratic formula is used to find the roots of a quadratic equation."),
			"hard", 15, strPtr(`["math", "algebra", "quadratic"]`), &catMath, teacherID2,
		},
		{
			"true-false", "Velocity",
			"Velocity is a scalar quantity that only measures speed.",
			nil, "false",
			strPtr("Velocity is a vector quantity that includes both speed and direction."),
			"easy", 3, strPtr(`["physics", "motion"]`), &catPhys, teacherID2,
		},
		{
			"essay", "Renewable Energy Essay",
			"Explain two renewable energy sources and their benefits (minimum 3 sentences).",
			nil,
			"Sample key: solar and wind; reduce fossil fuel reliance; lower emissions (graded manually).",
			strPtr("Essay questions require manual grading."),
			"medium", 15, strPtr(`["environment", "energy", "essay"]`), &catBio, teacherID1,
		},
		{
			"matching", "Provinces and Capitals",
			"Match: A=Aceh, B=Central Java, C=Papua with 1=Jayapura, 2=Banda Aceh, 3=Semarang. Answer format A-2,B-3,C-1.",
			strPtr(`["A. Aceh","B. Central Java","C. Papua","1. Jayapura","2. Banda Aceh","3. Semarang"]`),
			"A-2,B-3,C-1",
			strPtr("Matching is graded manually."),
			"medium", 10, strPtr(`["geography"]`), &catBio, teacherID1,
		},
		{
			"fill-blank", "Third Planet",
			"The third planet from the Sun is _____.",
			nil,
			"Earth",
			strPtr("Answer comparison is case-insensitive."),
			"easy", 5, strPtr(`["astronomy"]`), &catPhys, teacherID1,
		},
	}

	questionIDs := make([]string, len(questions))
	for i, q := range questions {
		var id string
		err := db.QueryRow(`
			INSERT INTO questions (type, title, content, options, correct_answer, explanation,
				difficulty, points, tags, category_id, created_by, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			RETURNING id
		`, q.type_, q.title, q.content, q.options, q.correctAns, q.explanation,
			q.difficulty, q.points, q.tags, q.categoryID, q.createdBy).Scan(&id)
		if err != nil {
			return fmt.Errorf("failed to insert question %s: %w", q.title, err)
		}
		questionIDs[i] = id
	}
	fmt.Println("✓ Questions seeded")

	// ===== QUESTION BANKS =====
	fmt.Println("Seeding Question Banks...")

	var bankID1, bankID2, bankID3 string
	banks := []struct {
		name        string
		description *string
		createdBy   string
		isPublic    bool
	}{
		{"Biology Questions", strPtr("Collection of biology questions for grades 9-12"), teacherID1, false},
		{"Physics Questions", strPtr("Physics problems covering mechanics and thermodynamics"), teacherID1, true},
		{"Mathematics Bank", strPtr("Algebra and calculus problems"), teacherID2, true},
	}

	for i, b := range banks {
		var id string
		err := db.QueryRow(`
			INSERT INTO question_banks (name, description, created_by, is_public, created_at, updated_at)
			VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			RETURNING id
		`, b.name, b.description, b.createdBy, b.isPublic).Scan(&id)
		if err != nil {
			return fmt.Errorf("failed to insert bank %s: %w", b.name, err)
		}

		if i == 0 { bankID1 = id }
		if i == 1 { bankID2 = id }
		if i == 2 { bankID3 = id }
	}
	fmt.Println("✓ Question Banks seeded")

	// Link questions to banks
	_, err := db.Exec(`
		INSERT INTO question_bank_questions (bank_id, question_id, order_index)
		VALUES ($1, $2, 0)
		ON CONFLICT (bank_id, question_id) DO NOTHING
	`, bankID1, questionIDs[0])
	if err != nil {
		return fmt.Errorf("failed to link questions to bank 1: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO question_bank_questions (bank_id, question_id, order_index)
		VALUES ($1, $2, 1)
		ON CONFLICT (bank_id, question_id) DO NOTHING
	`, bankID1, questionIDs[3])
	if err != nil {
		return fmt.Errorf("failed to link questions to bank 1b: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO question_bank_questions (bank_id, question_id, order_index)
		VALUES ($1, $2, 0)
		ON CONFLICT (bank_id, question_id) DO NOTHING
	`, bankID2, questionIDs[1])
	if err != nil {
		return fmt.Errorf("failed to link questions to bank 2: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO question_bank_questions (bank_id, question_id, order_index)
		VALUES ($1, $2, 1)
		ON CONFLICT (bank_id, question_id) DO NOTHING
	`, bankID2, questionIDs[7])
	if err != nil {
		return fmt.Errorf("failed to link questions to bank 2b: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO question_bank_questions (bank_id, question_id, order_index)
		VALUES ($1, $2, 0)
		ON CONFLICT (bank_id, question_id) DO NOTHING
	`, bankID3, questionIDs[4])
	if err != nil {
		return fmt.Errorf("failed to link questions to bank 3: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO question_bank_questions (bank_id, question_id, order_index)
		VALUES ($1, $2, 1)
		ON CONFLICT (bank_id, question_id) DO NOTHING
	`, bankID3, questionIDs[6])
	if err != nil {
		return fmt.Errorf("failed to link questions to bank 3b: %w", err)
	}
	fmt.Println("✓ Questions linked to banks")

	// ===== EXAMS =====
	fmt.Println("Seeding Exams...")

	examConfig := `{"duration":60,"shuffleQuestions":false,"shuffleOptions":true,"showResults":"after-review","allowReview":true,"maxAttempts":1,"passingScore":70}`

	var examID1, examID2, examID3, examID4 string
	exams := []struct {
		title         string
		description   *string
		subject       string
		grade         string
		status        string
		createdBy     string
		scheduledStart *time.Time
		scheduledEnd   *time.Time
	}{
		{
			"Biology Midterm Exam",
			strPtr("Comprehensive midterm covering cellular biology, genetics, and ecosystems."),
			"Biology", "10th Grade", "published", teacherID1,
			timePtr(time.Now().Add(24 * time.Hour)), timePtr(time.Now().Add(26 * time.Hour)),
		},
		{
			"Physics Quiz - Newton's Laws",
			strPtr("Quick quiz on Newton's three laws of motion."),
			"Physics", "11th Grade", "active", teacherID1,
			timePtr(time.Now().Add(-1 * time.Hour)), timePtr(time.Now().Add(1 * time.Hour)),
		},
		{
			"Chemistry Fundamentals",
			strPtr("Introduction to chemical bonds and reactions."),
			"Chemistry", "9th Grade", "draft", teacherID1,
			timePtr(time.Now().Add(48 * time.Hour)), timePtr(time.Now().Add(50 * time.Hour)),
		},
		{
			"Mathematics - Quadratic Equations",
			strPtr("Test your knowledge of quadratic equations and the quadratic formula."),
			"Mathematics", "11th Grade", "published", teacherID2,
			timePtr(time.Now().Add(72 * time.Hour)), timePtr(time.Now().Add(74 * time.Hour)),
		},
	}

	for i, e := range exams {
		var id string
		err := db.QueryRow(`
			INSERT INTO exams (title, description, subject, grade, config, status, created_by, scheduled_start, scheduled_end, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			RETURNING id
		`, e.title, e.description, e.subject, e.grade, examConfig, e.status, e.createdBy, e.scheduledStart, e.scheduledEnd).Scan(&id)
		if err != nil {
			return fmt.Errorf("failed to insert exam %s: %w", e.title, err)
		}

		if i == 0 { examID1 = id }
		if i == 1 { examID2 = id }
		if i == 2 { examID3 = id }
		if i == 3 { examID4 = id }
	}
	fmt.Println("✓ Exams seeded")

	// Link questions to exams
	_, err = db.Exec(`
		INSERT INTO exam_questions (exam_id, question_id, order_index, points)
		VALUES ($1, $2, 0, 5)
		ON CONFLICT (exam_id, question_id) DO NOTHING
	`, examID1, questionIDs[0])
	if err != nil {
		return fmt.Errorf("failed to link questions to exam 1a: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO exam_questions (exam_id, question_id, order_index, points)
		VALUES ($1, $2, 1, 8)
		ON CONFLICT (exam_id, question_id) DO NOTHING
	`, examID1, questionIDs[3])
	if err != nil {
		return fmt.Errorf("failed to link questions to exam 1b: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO exam_questions (exam_id, question_id, order_index, points)
		VALUES ($1, $2, 0, 10)
		ON CONFLICT (exam_id, question_id) DO NOTHING
	`, examID2, questionIDs[1])
	if err != nil {
		return fmt.Errorf("failed to link questions to exam 2a: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO exam_questions (exam_id, question_id, order_index, points)
		VALUES ($1, $2, 1, 3)
		ON CONFLICT (exam_id, question_id) DO NOTHING
	`, examID2, questionIDs[7])
	if err != nil {
		return fmt.Errorf("failed to link questions to exam 2b: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO exam_questions (exam_id, question_id, order_index, points)
		VALUES ($1, $2, 0, 3)
		ON CONFLICT (exam_id, question_id) DO NOTHING
	`, examID3, questionIDs[2])
	if err != nil {
		return fmt.Errorf("failed to link questions to exam 3a: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO exam_questions (exam_id, question_id, order_index, points)
		VALUES ($1, $2, 1, 5)
		ON CONFLICT (exam_id, question_id) DO NOTHING
	`, examID3, questionIDs[5])
	if err != nil {
		return fmt.Errorf("failed to link questions to exam 3b: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO exam_questions (exam_id, question_id, order_index, points)
		VALUES ($1, $2, 0, 10)
		ON CONFLICT (exam_id, question_id) DO NOTHING
	`, examID4, questionIDs[4])
	if err != nil {
		return fmt.Errorf("failed to link questions to exam 4a: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO exam_questions (exam_id, question_id, order_index, points)
		VALUES ($1, $2, 1, 15)
		ON CONFLICT (exam_id, question_id) DO NOTHING
	`, examID4, questionIDs[6])
	if err != nil {
		return fmt.Errorf("failed to link questions to exam 4b: %w", err)
	}
	fmt.Println("✓ Questions linked to exams")

	// ===== EXAM SESSIONS (simulated completed exams) =====
	fmt.Println("Seeding Exam Sessions...")

	var sessionID1, sessionID2, sessionID3 string
	now := time.Now()
	sessions := []struct {
		examID      string
		studentID   string
		answers     *string
		status      string
		startedAt   *time.Time
		submittedAt *time.Time
		timeSpent   int
		score       *int
		gradedAt    *time.Time
	}{
		{
			examID1, studentID1,
			strPtr(`{"q-1":"Oxygen","q-4":"Mitosis"}`),
			"graded",
			timePtr(now.Add(-2 * time.Hour)), timePtr(now.Add(-1 * time.Hour)),
			3600, intPtr(13), timePtr(now.Add(-55 * time.Minute)),
		},
		{
			examID1, studentID2,
			strPtr(`{"q-1":"Oxygen","q-4":"Mitosis"}`),
			"graded",
			timePtr(now.Add(-2 * time.Hour)), timePtr(now.Add(-1 * time.Hour)),
			3300, intPtr(13), timePtr(now.Add(-55 * time.Minute)),
		},
		{
			examID1, studentID3,
			strPtr(`{"q-1":"Carbon Dioxide","q-4":"Meiosis"}`),
			"graded",
			timePtr(now.Add(-2 * time.Hour)), timePtr(now.Add(-1 * time.Hour)),
			4000, intPtr(0), timePtr(now.Add(-55 * time.Minute)),
		},
	}

	for i, s := range sessions {
		var id string
		err := db.QueryRow(`
			INSERT INTO exam_sessions (exam_id, student_id, answers, status, started_at, submitted_at, time_spent, score, graded_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			ON CONFLICT (exam_id, student_id) DO UPDATE SET status = EXCLUDED.status
			RETURNING id
		`, s.examID, s.studentID, s.answers, s.status, s.startedAt, s.submittedAt, s.timeSpent, s.score, s.gradedAt).Scan(&id)
		if err != nil {
			return fmt.Errorf("failed to insert session: %w", err)
		}

		if i == 0 { sessionID1 = id }
		if i == 1 { sessionID2 = id }
		if i == 2 { sessionID3 = id }
	}
	fmt.Println("✓ Exam Sessions seeded")

	// ===== ANSWER REVIEWS =====
	fmt.Println("Seeding Answer Reviews...")

	reviews := []struct {
		sessionID  string
		questionID string
		studentAns string
		correctAns string
		isCorrect  bool
		points     int
		maxPoints  int
	}{
		{sessionID1, questionIDs[0], "Oxygen", "Oxygen", true, 5, 5},
		{sessionID1, questionIDs[3], "Mitosis", "Mitosis", true, 8, 8},
		{sessionID2, questionIDs[0], "Oxygen", "Oxygen", true, 5, 5},
		{sessionID2, questionIDs[3], "Mitosis", "Mitosis", true, 8, 8},
		{sessionID3, questionIDs[0], "Carbon Dioxide", "Oxygen", false, 0, 5},
		{sessionID3, questionIDs[3], "Meiosis", "Mitosis", false, 0, 8},
	}

	for _, r := range reviews {
		_, err := db.Exec(`
			INSERT INTO answer_reviews (session_id, question_id, student_answer, correct_answer, is_correct, points, max_points)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (session_id, question_id) DO NOTHING
		`, r.sessionID, r.questionID, r.studentAns, r.correctAns, r.isCorrect, r.points, r.maxPoints)
		if err != nil {
			return fmt.Errorf("failed to insert answer review: %w", err)
		}
	}
	fmt.Println("✓ Answer Reviews seeded")

	// ===== EXAM RESULTS =====
	fmt.Println("Seeding Exam Results...")

	studentNames := map[string]string{
		studentID1: "Ahmad Wijaya",
		studentID2: "Siti Rahayu",
		studentID3: "Budi Santoso",
	}

	results := []struct {
		sessionID   string
		examID      string
		studentID   string
		score       int
		maxScore    int
		passed      bool
		timeSpent   int
		submittedAt time.Time
		gradedAt    *time.Time
	}{
		{sessionID1, examID1, studentID1, 13, 13, true, 3600, now.Add(-1 * time.Hour), timePtr(now.Add(-55 * time.Minute))},
		{sessionID2, examID1, studentID2, 13, 13, true, 3300, now.Add(-1 * time.Hour), timePtr(now.Add(-55 * time.Minute))},
		{sessionID3, examID1, studentID3, 0, 13, false, 4000, now.Add(-1 * time.Hour), timePtr(now.Add(-55 * time.Minute))},
	}

	for _, r := range results {
		_, err := db.Exec(`
			INSERT INTO exam_results (session_id, exam_id, student_id, student_name, score, max_score, passed, time_spent, submitted_at, graded_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT DO NOTHING
		`, r.sessionID, r.examID, r.studentID, studentNames[r.studentID], r.score, r.maxScore, r.passed, r.timeSpent, r.submittedAt, r.gradedAt)
		if err != nil {
			return fmt.Errorf("failed to insert result: %w", err)
		}
	}
	fmt.Println("✓ Exam Results seeded")

	// ===== ACTIVITY LOG =====
	fmt.Println("Seeding Activity Log...")

	activities := []struct {
		userID   *string
		type_    string
		title    string
		metadata *string
	}{
		{&studentID1, "exam-submitted", "Exam Submitted", strPtr(`{"examTitle":"Biology Midterm Exam","studentName":"Ahmad Wijaya"}`)},
		{&teacherID1, "exam-created", "New Exam Created", strPtr(`{"examTitle":"Physics Quiz - Newton's Laws"}`)},
		{&teacherID1, "question-added", "Questions Added", strPtr(`{"count":5,"bankName":"Biology Questions"}`)},
		{&studentID2, "exam-submitted", "Exam Submitted", strPtr(`{"examTitle":"Biology Midterm Exam","studentName":"Siti Rahayu"}`)},
		{&teacherID1, "exam-graded", "Exams Graded", strPtr(`{"count":3,"examTitle":"Biology Midterm Exam"}`)},
	}

	for _, a := range activities {
		_, err := db.Exec(`
			INSERT INTO activity_log (user_id, type, title, description, metadata, created_at)
			VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP)
		`, a.userID, a.type_, a.title, strPtr(a.title), a.metadata)
		if err != nil {
			return fmt.Errorf("failed to insert activity: %w", err)
		}
	}
	fmt.Println("✓ Activity Log seeded")

	return nil
}
