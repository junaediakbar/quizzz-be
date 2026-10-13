package handlers

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// PageParams adalah parameter pagination yang sudah divalidasi.
//
// Limit == 0 berarti "tanpa batas" (perilaku lama dipertahankan supaya klien
// yang belum mengirim limit tetap mendapat seluruh data).
type PageParams struct {
	Limit  int
	Offset int
}

// MaxPageLimit membatasi jumlah baris per permintaan.
const MaxPageLimit = 500

// ParsePageParams membaca ?limit=&offset= dari query string.
//
// Sebelumnya nilai yang tidak valid (mis. "abc" atau "-5") diabaikan diam-diam
// sehingga seluruh tabel ikut terkirim; sekarang menjadi 400 yang jelas.
func ParsePageParams(c *fiber.Ctx) (PageParams, error) {
	var p PageParams

	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return p, fmt.Errorf("limit harus bilangan bulat >= 0")
		}
		if n > MaxPageLimit {
			n = MaxPageLimit
		}
		p.Limit = n
	}

	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return p, fmt.Errorf("offset harus bilangan bulat >= 0")
		}
		p.Offset = n
	}

	return p, nil
}

// ListMeta adalah amplop metadata pagination yang dipakai semua endpoint list.
//
//	count     = jumlah baris yang dikembalikan pada respons ini
//	total     = jumlah seluruh baris yang cocok dengan filter
//	has_more  = masih ada baris setelah halaman ini
type ListMeta struct {
	Count   int
	Total   int
	Limit   int
	Offset  int
	HasMore bool
}

// NewListMeta menghitung metadata dari jumlah baris yang dikembalikan.
func NewListMeta(returned, total int, p PageParams) ListMeta {
	return ListMeta{
		Count:   returned,
		Total:   total,
		Limit:   p.Limit,
		Offset:  p.Offset,
		HasMore: p.Offset+returned < total,
	}
}

// Apply menyalin metadata pagination ke dalam body respons.
func (m ListMeta) Apply(body fiber.Map) fiber.Map {
	body["count"] = m.Count
	body["total"] = m.Total
	body["limit"] = m.Limit
	body["offset"] = m.Offset
	body["has_more"] = m.HasMore
	return body
}

// PageError writes the standard 400 response for invalid pagination params.
func PageError(c *fiber.Ctx, err error) error {
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
}
