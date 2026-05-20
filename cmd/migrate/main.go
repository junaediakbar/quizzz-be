// Package main runs SQL migration files against DATABASE_URL (no psql required).
//
// Usage (from backend directory):
//
//	go run ./cmd/migrate
//
// Applies all numbered migrations migrations/NNN_*.sql in order (skips schema.sql).
//
// Single file:
//
//	go run ./cmd/migrate migrations/003_question_image_urls.sql
//
// If you run from the repository root (parent of backend/), paths are resolved automatically.
package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	_ = godotenv.Load()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required (set in .env or environment)")
		os.Exit(1)
	}

	baseDir, err := resolveBackendDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	migDir := filepath.Join(baseDir, "migrations")

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		fmt.Fprintln(os.Stderr, "ping:", err)
		os.Exit(1)
	}

	var files []string
	if len(os.Args) > 1 {
		arg := os.Args[1]
		p := arg
		if !filepath.IsAbs(p) && !strings.HasPrefix(p, "."+string(filepath.Separator)) {
			try := filepath.Join(migDir, arg)
			if _, statErr := os.Stat(try); statErr == nil {
				p = try
			} else if _, statErr := os.Stat(filepath.Join(baseDir, arg)); statErr == nil {
				p = filepath.Join(baseDir, arg)
			}
		}
		files = []string{p}
	} else {
		files, err = listNumberedMigrations(migDir)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if len(files) == 0 {
			fmt.Fprintln(os.Stderr, "no migrations found in", migDir)
			os.Exit(1)
		}
	}

	for _, path := range files {
		sqlBytes, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := execSQLScript(db, string(sqlBytes)); err != nil {
			fmt.Fprintf(os.Stderr, "migrate %s: %v\n", path, err)
			os.Exit(1)
		}
		rel, _ := filepath.Rel(baseDir, path)
		if rel == "." {
			rel = path
		}
		fmt.Println("Applied:", rel)
	}
}

// resolveBackendDir finds the folder containing go.mod and migrations/.
// Works when cwd is backend/, repo root with a backend/ subfolder, or a few levels deep.
func resolveBackendDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	type pair struct{ base string }
	var candidates []pair
	dir := wd
	seen := map[string]bool{}
	for i := 0; i < 10; i++ {
		for _, base := range []string{dir, filepath.Join(dir, "backend")} {
			if seen[base] {
				continue
			}
			seen[base] = true
			candidates = append(candidates, pair{base: base})
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	for _, c := range candidates {
		if _, e := os.Stat(filepath.Join(c.base, "go.mod")); e != nil {
			continue
		}
		if st, e := os.Stat(filepath.Join(c.base, "migrations")); e == nil && st.IsDir() {
			return c.base, nil
		}
	}
	return "", fmt.Errorf("could not find backend (go.mod + migrations/). Try: cd backend && go run ./cmd/migrate")
}

var numberedMigration = regexp.MustCompile(`^\d{3}_.*\.sql$`)

func listNumberedMigrations(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "schema.sql" || !strings.HasSuffix(name, ".sql") {
			continue
		}
		if numberedMigration.MatchString(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, filepath.Join(dir, n))
	}
	return out, nil
}

// execSQLScript runs statements, properly handling PostgreSQL dollar-quoted strings.
func execSQLScript(db *sql.DB, sqlContent string) error {
	sqlContent = strings.TrimSpace(sqlContent)
	if sqlContent == "" {
		return nil
	}

	// For schema.sql, execute as a single multi-statement script
	// For other migrations, split by semicolon but respect $$ quotes
	if strings.Contains(sqlContent, "CREATE OR REPLACE FUNCTION") || strings.Contains(sqlContent, "AS $$") {
		// Execute the whole script at once for complex SQL with $$ quotes
		_, err := db.Exec(sqlContent)
		if err != nil {
			return fmt.Errorf("%w", err)
		}
		return nil
	}

	// Simple split by semicolon for regular migrations
	parts := strings.Split(sqlContent, ";")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Skip pure comment lines
		if strings.HasPrefix(p, "--") {
			continue
		}
		if _, err := db.Exec(p); err != nil {
			snippet := p
			if len(snippet) > 400 {
				snippet = snippet[:400] + "..."
			}
			return fmt.Errorf("%w\n--- statement ---\n%s", err, snippet)
		}
	}
	return nil
}
