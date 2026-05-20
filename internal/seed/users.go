package seed

import (
	"database/sql"
	"fmt"

	"github.com/junaediakbar/quizzz-backend/internal/services"
)

// Akun demo bersama password DemoPassword (lihat konstanta di e2e.go).
const (
	EmailTeacher   = "teacher@seed.quizzz.dev"
	EmailStudent   = "student@seed.quizzz.dev"
	EmailStudent2  = "student2@seed.quizzz.dev"
	EmailStudent3  = "student3@seed.quizzz.dev"
	EmailAdmin     = "admin@seed.quizzz.dev"
	DemoPassword   = "password123"
)

// DemoUserSpec satu baris untuk seed / upsert.
type DemoUserSpec struct {
	Name  string
	Email string
	Role  string
}

// DemoUserSpecs daftar akun @seed.quizzz.dev (dipakai RunE2E dan InsertDemoUsers).
func DemoUserSpecs() []DemoUserSpec {
	return []DemoUserSpec{
		{Name: "Pak Guru Seed", Email: EmailTeacher, Role: "teacher"},
		{Name: "Budi Murid", Email: EmailStudent, Role: "student"},
		{Name: "Ani Murid", Email: EmailStudent2, Role: "student"},
		{Name: "Citra Murid", Email: EmailStudent3, Role: "student"},
		{Name: "Admin Seed", Email: EmailAdmin, Role: "admin"},
	}
}

// InsertDemoUsers meng-upsert akun demo (email unik). Password sama untuk semua: DemoPassword.
// Aman dipanggil berulang (ON CONFLICT email).
func InsertDemoUsers(db *sql.DB) error {
	hash, err := services.HashPassword(DemoPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	for _, u := range DemoUserSpecs() {
		_, err := db.Exec(`
			INSERT INTO users (id, name, email, password_hash, role, created_at, updated_at)
			VALUES (uuid_generate_v4(), $1, $2, $3, $4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			ON CONFLICT (email) DO UPDATE SET
				name = EXCLUDED.name,
				password_hash = EXCLUDED.password_hash,
				role = EXCLUDED.role,
				updated_at = CURRENT_TIMESTAMP
		`, u.Name, u.Email, hash, u.Role)
		if err != nil {
			return fmt.Errorf("user %s: %w", u.Email, err)
		}
	}
	return nil
}

// PrintDemoUserBanner mencetak kredensial ke stdout (dipakai cmd/seed -users).
func PrintDemoUserBanner() {
	fmt.Println()
	fmt.Println("=== Akun demo user (@seed.quizzz.dev) ===")
	fmt.Printf("Password untuk semua: %s\n", DemoPassword)
	for _, u := range DemoUserSpecs() {
		fmt.Printf("  %-8s  %s  (%s)\n", u.Role, u.Email, u.Name)
	}
	fmt.Println()
}
