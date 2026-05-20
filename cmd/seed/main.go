// Seed inserts demo users, questions, exams, and one completed session (see internal/seed).
// Requires DATABASE_URL and applied migrations (including exam_proctoring_events).
//
// Usage:
//
//	cd backend && go run ./cmd/seed
//	cd backend && go run ./cmd/seed -reset   # kosongkan DB lalu seed E2E
//	cd backend && go run ./cmd/seed -users   # hanya upsert akun demo
//	cd backend && go run ./cmd/seed -reset -users   # DB kosong + hanya user demo
package main

import (
	"flag"
	"log"

	"github.com/joho/godotenv"

	"github.com/junaediakbar/quizzz-backend/internal/database"
	"github.com/junaediakbar/quizzz-backend/internal/seed"
)

func main() {
	reset := flag.Bool("reset", false, "truncate semua tabel aplikasi lalu jalankan seed E2E (bersihkan data lama)")
	usersOnly := flag.Bool("users", false, "hanya upsert user demo (@seed.quizzz.dev); tanpa soal/ujian/sesi")
	flag.Parse()

	_ = godotenv.Load()

	if err := database.Connect(); err != nil {
		log.Fatal(err)
	}
	defer func() { _ = database.Close() }()

	db := database.GetDB()
	if *usersOnly {
		if *reset {
			if err := seed.TruncateAll(db); err != nil {
				log.Fatal(err)
			}
			log.Println("Semua data aplikasi dikosongkan.")
		}
		if err := seed.InsertDemoUsers(db); err != nil {
			log.Fatal(err)
		}
		seed.PrintDemoUserBanner()
		return
	}

	if *reset {
		if err := seed.TruncateAll(db); err != nil {
			log.Fatal(err)
		}
		log.Println("Semua data aplikasi dikosongkan; melanjutkan seed E2E…")
	}

	if err := seed.RunE2E(db); err != nil {
		log.Fatal(err)
	}
}
