package database

import (
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	_ "github.com/lib/pq"
)

// ensurePQPoolerCompat appends query flags so github.com/lib/pq works with PgBouncer /
// Neon pooled connections (transaction mode). Without this, concurrent requests can hit:
// "pq: unnamed prepared statement does not exist" or "bind message has N result formats...".
// See: https://github.com/lib/pq/issues/889
func ensurePQPoolerCompat(dsn string) string {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return dsn
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme == "" {
		return dsn
	}
	q := u.Query()
	if q.Get("binary_parameters") == "" {
		q.Set("binary_parameters", "yes")
	}
	u.RawQuery = q.Encode()
	return u.String()
}

var DB *sql.DB

// Connect establishes a connection to the PostgreSQL database
func Connect() error {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL environment variable is not set")
	}
	dbURL = ensurePQPoolerCompat(dbURL)

	var err error
	DB, err = sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err = DB.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// Set connection pool settings
	DB.SetMaxOpenConns(25)
	DB.SetMaxIdleConns(5)

	log.Println("Database connected successfully")
	return nil
}

// Close closes the database connection
func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}

// GetDB returns the database instance
func GetDB() *sql.DB {
	return DB
}
