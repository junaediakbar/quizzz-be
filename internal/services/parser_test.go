package services

import (
	"encoding/json"
	"testing"
)

func findQ(t *testing.T, res *ParseResult, idx int) ParsedQuestion {
	t.Helper()
	if res == nil || !res.Success {
		t.Fatalf("parse failed: %+v", res)
	}
	if idx >= len(res.Questions) {
		t.Fatalf("question %d missing, got %d questions (%+v)", idx, len(res.Questions), res.Questions)
	}
	return res.Questions[idx]
}

func TestParseStepByStepMarkerFormat(t *testing.T) {
	text := `1. Tentukan x dari 3x + 5 = 20.
Langkah 1: Kurangkan kedua ruas dengan 5 | Kunci: 3x = 15
Langkah 2: Bagi kedua ruas dengan 3 | Kunci: x = 5`

	res := ParseQuestionsSimple(text)
	if len(res.Questions) != 1 {
		t.Fatalf("expected 1 question, got %d: %+v", len(res.Questions), res.Questions)
	}
	q := findQ(t, res, 0)
	if q.Type != "step-by-step" {
		t.Fatalf("type = %q, want step-by-step", q.Type)
	}
	if len(q.Options) != 2 || q.Options[1] != "Bagi kedua ruas dengan 3" {
		t.Errorf("steps = %+v", q.Options)
	}
	var keys []string
	if err := json.Unmarshal([]byte(q.CorrectAnswer), &keys); err != nil {
		t.Fatalf("answer key is not a JSON array: %q: %v", q.CorrectAnswer, err)
	}
	if len(keys) != 2 || keys[0] != "3x = 15" || keys[1] != "x = 5" {
		t.Errorf("keys = %+v", keys)
	}
}

func TestParseNumberedFormatWithImageMarkers(t *testing.T) {
	text := `[!above https://cdn.example.com/diagram.png]
1. 5² dibaca ....
a) Lima akar dua
b) Dua pangkat lima
c) Lima kuadrat
d) Dua kuadrat
Correct answer: c

2. The chemical formula for water is H2O. True or False?
Correct answer: True`

	res := ParseQuestionsSimple(text)
	if len(res.Questions) != 2 {
		t.Fatalf("expected 2 questions, got %d: %+v", len(res.Questions), res.Questions)
	}
	q1 := findQ(t, res, 0)
	if q1.Content != "5² dibaca ...." {
		t.Errorf("q1 content = %q", q1.Content)
	}
	if len(q1.Options) != 4 || q1.Options[2] != "Lima kuadrat" {
		t.Errorf("q1 options = %+v", q1.Options)
	}
	if q1.CorrectAnswer != "Lima kuadrat" {
		t.Errorf("q1 answer = %q (want option text)", q1.CorrectAnswer)
	}
	if len(q1.ImageURLs) != 1 || q1.ImageURLs[0] != "https://cdn.example.com/diagram.png" {
		t.Errorf("q1 images = %+v", q1.ImageURLs)
	}

	q2 := findQ(t, res, 1)
	if q2.Type != "true-false" {
		t.Errorf("q2 type = %q, want true-false", q2.Type)
	}
	if q2.CorrectAnswer != "true" {
		t.Errorf("q2 answer = %q, want true", q2.CorrectAnswer)
	}
}

func TestParseMarkdownAuthoringFormat(t *testing.T) {
	text := `# Bank Soal — IPAS Kelas V — Bab 8

- Kelas: V
- Mapel: IPAS
- Bab: 8 — Ramai karena Bunyi

---

**[PG-001]** Perhatikan gambar gitar di atas. Bunyi pada gitar dihasilkan karena ....
A. senar dipukul dengan stik
B. senar bergetar saat dipetik
C. senar ditiup angin
D. senar dipanaskan matahari
- Kunci: B
- Pembahasan: Senar gitar bergetar saat dipetik.
- Tingkat: sulit
- gambar: https://upload.wikimedia.org/x/gitar.png
- credit: Wikimedia Commons

**[IS-001]** Ibu kota Provinsi Jawa Barat adalah ....
- Kunci: Bandung

**[BS-001]** Pernyataan: "Air mengalir dari tempat tinggi ke tempat rendah."
- Kunci: Benar`

	res := ParseQuestionsSimple(text)
	if len(res.Questions) != 3 {
		t.Fatalf("expected 3 questions, got %d: %+v", len(res.Questions), res.Questions)
	}

	q1 := findQ(t, res, 0)
	if q1.Type != "multiple-choice" {
		t.Errorf("q1 type = %q", q1.Type)
	}
	if q1.Content != "Perhatikan gambar gitar di atas. Bunyi pada gitar dihasilkan karena ...." {
		t.Errorf("q1 content = %q", q1.Content)
	}
	if len(q1.Options) != 4 || q1.Options[1] != "senar bergetar saat dipetik" {
		t.Errorf("q1 options = %+v", q1.Options)
	}
	if q1.CorrectAnswer != "senar bergetar saat dipetik" {
		t.Errorf("q1 answer = %q", q1.CorrectAnswer)
	}
	if q1.Explanation != "Senar gitar bergetar saat dipetik." {
		t.Errorf("q1 explanation = %q", q1.Explanation)
	}
	if q1.Difficulty != "hard" {
		t.Errorf("q1 difficulty = %q, want hard", q1.Difficulty)
	}
	if len(q1.ImageURLs) != 1 {
		t.Errorf("q1 images = %+v", q1.ImageURLs)
	}

	q2 := findQ(t, res, 1)
	if q2.Type != "fill-blank" {
		t.Errorf("q2 type = %q, want fill-blank", q2.Type)
	}
	if q2.CorrectAnswer != "Bandung" {
		t.Errorf("q2 answer = %q", q2.CorrectAnswer)
	}

	q3 := findQ(t, res, 2)
	if q3.Type != "true-false" {
		t.Errorf("q3 type = %q, want true-false", q3.Type)
	}
	if q3.CorrectAnswer != "true" {
		t.Errorf("q3 answer = %q, want true (Benar is mapped)", q3.CorrectAnswer)
	}
}

func TestParseSectionsIntoParts(t *testing.T) {
	text := `I. Pilihlah jawaban yang paling tepat!
Berilah tanda silang (X) pada huruf a, b, c, atau d!

1. Ibu kota Indonesia adalah ....
a) Bandung
b) Jakarta
Correct answer: b

2. 2 + 3 = ....
a) 4
b) 5
Correct answer: b

II. Uraian
Jawablah pertanyaan berikut dengan jelas!

3. Jelaskan proses fotosintesis.
Kunci: Tumbuhan mengubah cahaya matahari menjadi energi.

Bagian C: Isian Singkat
4. Air membeku pada suhu ....
Kunci: 0 derajat Celsius`

	res := ParseQuestionsSimple(text)
	if len(res.Questions) != 4 {
		t.Fatalf("expected 4 questions, got %d: %+v", len(res.Questions), res.Questions)
	}
	if len(res.Parts) != 3 {
		t.Fatalf("expected 3 parts, got %d: %+v", len(res.Parts), res.Parts)
	}
	if res.Parts[0].Title != "I. Pilihlah jawaban yang paling tepat!" {
		t.Errorf("part 1 = %q", res.Parts[0].Title)
	}
	if res.Parts[1].Title != "II. Uraian" {
		t.Errorf("part 2 = %q", res.Parts[1].Title)
	}
	if res.Parts[2].Title != "Bagian C: Isian Singkat" {
		t.Errorf("part 3 = %q", res.Parts[2].Title)
	}

	if findQ(t, res, 0).Part != res.Parts[0].Title || findQ(t, res, 1).Part != res.Parts[0].Title {
		t.Errorf("q1/q2 should be in part 1: %q / %q", res.Questions[0].Part, res.Questions[1].Part)
	}
	if findQ(t, res, 2).Part != res.Parts[1].Title {
		t.Errorf("q3 part = %q", res.Questions[2].Part)
	}
	if findQ(t, res, 3).Part != res.Parts[2].Title {
		t.Errorf("q4 part = %q", res.Questions[3].Part)
	}
	if findQ(t, res, 2).Type != "essay" {
		t.Errorf("q3 type = %q, want essay (in an Uraian section)", res.Questions[2].Type)
	}
}

func TestParseBareStemWithoutNumbering(t *testing.T) {
	text := `Apa ibukota Provinsi Jawa Barat?
a) Semarang
b) Bandung
Kunci: b`

	res := ParseQuestionsSimple(text)
	if len(res.Questions) != 1 {
		t.Fatalf("expected 1 question, got %d: %+v", len(res.Questions), res.Questions)
	}
	q := findQ(t, res, 0)
	if q.CorrectAnswer != "Bandung" {
		t.Errorf("answer = %q, want Bandung", q.CorrectAnswer)
	}
	if q.Type != "multiple-choice" || len(q.Options) != 2 {
		t.Errorf("type/options = %q/%+v", q.Type, q.Options)
	}
}

func TestParseMarkdownHeaderIsNotAQuestion(t *testing.T) {
	text := `# Bank Soal — Matematika Kelas V
- Kelas: V
- Mapel: Matematika

1. Hasil dari 2 + 2 adalah ....
a) 3
b) 4
Kunci: b`

	res := ParseQuestionsSimple(text)
	if len(res.Questions) != 1 {
		t.Fatalf("expected 1 question, got %d: %+v", len(res.Questions), res.Questions)
	}
	if res.Questions[0].Content != "Hasil dari 2 + 2 adalah ...." {
		t.Errorf("content = %q", res.Questions[0].Content)
	}
}

func TestNormalizeConvertsOptionlessMCQ(t *testing.T) {
	var warnings []string
	in := []ParsedQuestion{{
		Type:          "multiple-choice",
		Content:       "Jelaskan apa itu fotosintesis.",
		CorrectAnswer: "Proses tumbuhan mengubah cahaya menjadi energi",
	}}
	out := normalizeParsedQuestions(in, &warnings)
	if len(out) != 1 {
		t.Fatalf("expected 1 question, got %d", len(out))
	}
	if out[0].Type != "essay" {
		t.Errorf("type = %q, want essay for a 'Jelaskan' stem", out[0].Type)
	}
	if len(warnings) == 0 {
		t.Errorf("expected a warning about the missing options")
	}
}

func TestNormalizeDefaults(t *testing.T) {
	var warnings []string
	in := []ParsedQuestion{{
		Type:          "MCQ",
		Content:       "2 + 2 = ....",
		Options:       []string{" 3 ", "", "4"},
		CorrectAnswer: "(b)",
		Difficulty:    "sulit",
	}}
	out := normalizeParsedQuestions(in, &warnings)
	q := out[0]
	if q.Type != "multiple-choice" {
		t.Errorf("type = %q", q.Type)
	}
	if len(q.Options) != 2 || q.Options[1] != "4" {
		t.Errorf("options = %+v", q.Options)
	}
	if q.CorrectAnswer != "4" {
		t.Errorf("answer = %q, want 4", q.CorrectAnswer)
	}
	if q.Difficulty != "hard" {
		t.Errorf("difficulty = %q", q.Difficulty)
	}
	if q.Points <= 0 {
		t.Errorf("points should default, got %d", q.Points)
	}
}

func TestExtractJSONArray(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", `[{"a":1}]`, `[{"a":1}]`},
		{"fenced", "```json\n[{\"a\":1}]\n```", `[{"a":1}]`},
		{"prose", "Here you go:\n[{\"a\":1}]\nHope that helps!", `[{"a":1}]`},
		{"wrapper", `{"questions":[{"a":1}]}`, `[{"a":1}]`},
		{"brackets in string", `[{"a":"[not json]"}]`, `[{"a":"[not json]"}]`},
	}
	for _, tc := range cases {
		got, ok := extractJSONArray(tc.in)
		if !ok {
			t.Errorf("%s: not found", tc.name)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestRepairJSONTrailingComma(t *testing.T) {
	var qs []ParsedQuestion
	raw := `[{"type":"multiple-choice","content":"x","options":["a","b",],},]`
	if err := json.Unmarshal([]byte(repairJSON(raw)), &qs); err != nil {
		t.Fatalf("repair failed: %v", err)
	}
	if len(qs) != 1 || len(qs[0].Options) != 2 {
		t.Fatalf("unexpected result: %+v", qs)
	}
}

func TestParseCompactInlineOptionsAndMetadata(t *testing.T) {
	text := `### Bagian 1 — Latar Belakang

**[PG-021]** (sedang, bergambar: ` + "`peta.png`" + `) Perhatikan gambar! Ibukota Indonesia adalah ....
- A. Bandung — B. Jakarta — C. Surabaya — D. Medan
- Kunci: B — Pembahasan: Jakarta adalah ibukota Indonesia.

**[PG-022]** (mudah) Soal tanpa opsi di berkas draf.
- Kunci: jawaban singkat`

	res := ParseQuestionsSimple(text)
	if len(res.Questions) != 2 {
		t.Fatalf("expected 2 questions, got %d: %+v", len(res.Questions), res.Questions)
	}
	q1 := res.Questions[0]
	if q1.Content != "Perhatikan gambar! Ibukota Indonesia adalah ...." {
		t.Errorf("metadata prefix not stripped: %q", q1.Content)
	}
	if len(q1.Options) != 4 || q1.Options[1] != "Jakarta" {
		t.Errorf("inline options = %+v", q1.Options)
	}
	if q1.CorrectAnswer != "Jakarta" {
		t.Errorf("answer = %q", q1.CorrectAnswer)
	}
	if q1.Explanation != "Jakarta adalah ibukota Indonesia." {
		t.Errorf("explanation = %q", q1.Explanation)
	}
	if q1.Part != "Bagian 1 — Latar Belakang" {
		t.Errorf("part = %q", q1.Part)
	}
	if res.Questions[1].CorrectAnswer != "jawaban singkat" {
		t.Errorf("q2 answer = %q", res.Questions[1].CorrectAnswer)
	}
}

func TestParseCommaTag(t *testing.T) {
	text := `1. **[PG-001, mudah, bergambar mulut]** Pencernaan mekanis terjadi di ....
a) lambung
b) mulut
Kunci: b`
	res := ParseQuestionsSimple(text)
	if len(res.Questions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(res.Questions))
	}
	q := res.Questions[0]
	if q.Type != "multiple-choice" {
		t.Errorf("type = %q (tag with commas should map to multiple-choice)", q.Type)
	}
	if len(q.Options) != 2 || q.CorrectAnswer != "mulut" {
		t.Errorf("options/answer = %+v / %q", q.Options, q.CorrectAnswer)
	}
}

func TestParseAnswerAndExplanationOnSeparateLines(t *testing.T) {
	text := `1. 2 + 2 = ....
a) 3
b) 4
Kunci: b
Pembahasan: karena 2 + 2 = 4.`
	res := ParseQuestionsSimple(text)
	q := res.Questions[0]
	if q.CorrectAnswer != "4" || q.Explanation != "karena 2 + 2 = 4." {
		t.Errorf("answer=%q explanation=%q", q.CorrectAnswer, q.Explanation)
	}
}

func TestManualModeSkipsAI(t *testing.T) {
	text := "1. Ibu kota Indonesia adalah ....\na) Bandung\nb) Jakarta\nKunci: b"
	res, err := ParseQuestionsWithAI(text, nil, ParseModeManual)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Engine != "manual" {
		t.Errorf("engine = %q, want manual", res.Engine)
	}
	if len(res.Questions) != 1 || res.Questions[0].CorrectAnswer != "Jakarta" {
		t.Fatalf("parse manual gagal: %+v", res.Questions)
	}
}

func TestManualModeRejectsImages(t *testing.T) {
	res, err := ParseQuestionsWithAI("teks", []string{"data:image/png;base64,iVBORw0KGgo="}, ParseModeManual)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success || len(res.Errors) == 0 {
		t.Fatalf("mode manual dengan gambar harus gagal dengan pesan jelas: %+v", res)
	}
}

func TestTitleIsLabelNotCopyOfContent(t *testing.T) {
	text := `1. Dania membuat 60 kue nastar dan 72 kue kacang. Ia mengemas kue-kue itu ke dalam beberapa kotak dengan isi sama banyak.
a) 6
b) 12
Kunci: b`
	res := ParseQuestionsSimple(text)
	if len(res.Questions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(res.Questions))
	}
	q := res.Questions[0]
	if q.Title != "Soal 1" {
		t.Errorf("title = %q, want %q (judul tidak boleh menyalin isi soal)", q.Title, "Soal 1")
	}
	if titleRepeatsContent(q.Title, q.Content) {
		t.Errorf("judul masih dianggap mengulang isi")
	}
}

func TestTitleRepeatsContentDetectsDuplicate(t *testing.T) {
	content := "Dania membuat 60 kue nastar dan 72 kue kacang untuk acara sekolah."
	if !titleRepeatsContent(content, content) {
		t.Errorf("judul identik harus terdeteksi")
	}
	if !titleRepeatsContent("Dania membuat 60 kue nastar dan 72 kue", content) {
		t.Errorf("potongan awal isi harus terdeteksi")
	}
	if titleRepeatsContent("Soal 1", content) {
		t.Errorf("label pendek bukan duplikat")
	}
}

func TestFallbackTitlesAreNumbered(t *testing.T) {
	text := `1. Soal pertama yang cukup panjang untuk menguji penomoran judul.
a) satu
b) dua
Kunci: a

2. Soal kedua yang cukup panjang untuk menguji penomoran judul.
a) tiga
b) empat
Kunci: b

3. Soal ketiga yang cukup panjang untuk menguji penomoran judul.
a) lima
b) enam
Kunci: a`
	res := ParseQuestionsSimple(text)
	if len(res.Questions) != 3 {
		t.Fatalf("expected 3 questions, got %d", len(res.Questions))
	}
	for i, want := range []string{"Soal 1", "Soal 2", "Soal 3"} {
		if res.Questions[i].Title != want {
			t.Errorf("q%d title = %q, want %q", i+1, res.Questions[i].Title, want)
		}
	}
	if res.Engine != "manual" {
		t.Errorf("engine = %q, want manual", res.Engine)
	}
}

func TestThinkingControlDefaults(t *testing.T) {
	if thinkingEnabled() {
		t.Error("mode thinking harus NONAKTIF secara default (menyebabkan timeout di produksi)")
	}
	if !supportsThinkingControl("glm-4.5-flash") || !supportsThinkingControl("GLM-4.5V") {
		t.Error("model glm-4.5* harus menerima parameter thinking")
	}
	if supportsThinkingControl("glm-4-flash") || supportsThinkingControl("glm-3-turbo") {
		t.Error("model non-reasoning tidak boleh dikirimi parameter thinking")
	}
	t.Setenv("AI_PARSER_THINKING", "on")
	if !thinkingEnabled() {
		t.Error("AI_PARSER_THINKING=on harus mengaktifkan thinking")
	}
}
