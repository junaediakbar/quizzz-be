package services

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
)

// AIConfig holds AI service configuration.
// Provider is "zhipu" (Zhipu GLM / Z.AI-style BigModel key) or "gemini" (Google Gemini OpenAI-compatible API).
type AIConfig struct {
	BaseURL  string
	APIKey   string
	Model    string
	Provider string
}

// GetAIConfig returns AI configuration from environment.
//
// AI_PARSER_PROVIDER: zhipu | gemini | auto (default auto).
// auto prefers GEMINI_API_KEY when set, otherwise ZAI_API_KEY (Zhipu/BigModel).
func GetAIConfig() AIConfig {
	_ = godotenv.Load()

	provider := strings.ToLower(strings.TrimSpace(os.Getenv("AI_PARSER_PROVIDER")))
	geminiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	zaiKey := strings.TrimSpace(os.Getenv("ZAI_API_KEY"))

	if provider == "" || provider == "auto" {
		if geminiKey != "" {
			provider = "gemini"
		} else if zaiKey != "" {
			provider = "zhipu"
		} else {
			provider = ""
		}
	}

	baseURL := strings.TrimSpace(os.Getenv("AI_PARSER_BASE_URL"))
	model := strings.TrimSpace(os.Getenv("AI_PARSER_MODEL"))

	switch provider {
	case "openai":
		// Gateway OpenAI-compatible (mis. proxy): kirim Bearer key mentah, bukan JWT Zhipu.
		// Tanpa base URL, AI dinonaktifkan (kunci dikosongkan).
		apiKey := strings.TrimSpace(os.Getenv("AI_PARSER_API_KEY"))
		if baseURL == "" {
			apiKey = ""
		}
		return AIConfig{
			BaseURL:  strings.TrimSuffix(baseURL, "/"),
			APIKey:   apiKey,
			Model:    model,
			Provider: "openai",
		}
	case "gemini":
		if baseURL == "" {
			baseURL = "https://generativelanguage.googleapis.com/v1beta/openai"
		}
		if model == "" {
			model = "gemini-2.0-flash"
		}
		return AIConfig{
			BaseURL:  strings.TrimSuffix(baseURL, "/"),
			APIKey:   geminiKey,
			Model:    model,
			Provider: "gemini",
		}
	default:
		if baseURL == "" {
			baseURL = "https://open.bigmodel.cn/api/paas/v4"
		}
		if model == "" {
			// glm-3-turbo / glm-4-flash sudah dihentikan untuk banyak akun;
			// glm-4.5-flash tersedia luas dan gratis di BigModel.
			model = "glm-4.5-flash"
		}
		return AIConfig{
			BaseURL:  strings.TrimSuffix(baseURL, "/"),
			APIKey:   zaiKey,
			Model:    model,
			Provider: "zhipu",
		}
	}
}

// generateZhipuToken generates a JWT token for Zhipu AI API authentication.
//
// The BigModel/Zhipu API rejects the token unless the JWT *header* carries
// "sign_type":"SIGN" (see https://open.bigmodel.cn/dev/api#authentication) — a
// plain {"alg":"HS256","typ":"JWT"} header returns
// 401 "令牌已过期或验证不正确" (token expired or invalid).
func generateZhipuToken(apiKey string, expSeconds int) (string, error) {
	parts := strings.Split(apiKey, ".")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid API key format")
	}

	apiID := parts[0]
	apiSecret := parts[1]

	now := time.Now()
	exp := now.Add(time.Duration(expSeconds) * time.Second)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"api_key":   apiID,
		"exp":       exp.Unix(),
		"timestamp": now.Unix(),
	})
	token.Header["sign_type"] = "SIGN"

	// Sign with API secret
	tokenString, err := token.SignedString([]byte(apiSecret))
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}

// ChatResponse represents the API response
type ChatResponse struct {
	ID      string `json:"id"`
	Created int    `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
			// Reasoning models (glm-4.5, deepseek-r1, …) may return their text here.
			ReasoningContent string `json:"reasoning_content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func pickVisionChatModel(config AIConfig) string {
	if vm := strings.TrimSpace(os.Getenv("AI_PARSER_VISION_MODEL")); vm != "" {
		return vm
	}
	if config.Provider == "gemini" || config.Provider == "openai" {
		return config.Model
	}
	// Zhipu/BigModel vision model (glm-4v-flash dihentikan untuk banyak akun).
	return "glm-4.5v"
}

// validateImageDataURLs validates data:image/...;base64,... payloads (max 8 images, 4MB decoded each).
func validateImageDataURLs(urls []string) error {
	const maxN = 8
	const maxBytes = 4 * 1024 * 1024
	if len(urls) > maxN {
		return fmt.Errorf("too many images (max %d)", maxN)
	}
	for _, u := range urls {
		if len(u) > 25*1024*1024 {
			return fmt.Errorf("image payload too large")
		}
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(u)), "data:image/") {
			return fmt.Errorf("images must be data:image/…;base64,... URLs")
		}
		semi := strings.Index(strings.ToLower(u), ";base64,")
		if semi < 0 {
			return fmt.Errorf("invalid data URL (expected ;base64,)")
		}
		meta := strings.ToLower(strings.TrimSpace(u[len("data:"):semi]))
		allowed := meta == "image/jpeg" || meta == "image/jpg" || meta == "image/png" ||
			meta == "image/webp" || meta == "image/gif"
		if !allowed {
			return fmt.Errorf("only jpeg, png, webp, gif are allowed")
		}
		b64 := u[semi+len(";base64,"):]
		raw, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return fmt.Errorf("invalid base64 image data")
		}
		if len(raw) > maxBytes {
			return fmt.Errorf("each image must be at most 4 MB")
		}
	}
	return nil
}

// thinkingEnabled melaporkan apakah mode "berpikir" model reasoning diaktifkan.
//
// Default NONAKTIF. Model reasoning (glm-4.5+) bisa berpikir sangat lama untuk
// tugas ekstraksi JSON terstruktur; terukur di produksi: thinking aktif =
// timeout >120 dtk, thinking nonaktif = ~3,5 dtk dengan hasil JSON yang sama.
func thinkingEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AI_PARSER_THINKING"))) {
	case "on", "true", "1", "enabled", "yes":
		return true
	}
	return false
}

// supportsThinkingControl melaporkan apakah model menerima parameter `thinking`.
func supportsThinkingControl(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	for _, prefix := range []string{"glm-4.5", "glm-4.6", "glm-z1", "glm-5"} {
		if strings.HasPrefix(m, prefix) {
			return true
		}
	}
	return false
}

// doChatRequest mengirim payload ke endpoint chat/completions dan mengembalikan
// response beserta body-nya (body dibaca sekali agar bisa diulang tanpa param).
func doChatRequest(config AIConfig, payload map[string]interface{}, timeout time.Duration) (*http.Response, []byte, error) {
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	req, err := http.NewRequest("POST", config.BaseURL+"/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	switch strings.ToLower(config.Provider) {
	case "gemini", "openai":
		req.Header.Set("Authorization", "Bearer "+config.APIKey)
	default:
		token, err := generateZhipuToken(config.APIKey, 3600)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, body, nil
}

// postChatCompletion sends OpenAI-compatible messages (user content string or multimodal parts).
func postChatCompletion(config AIConfig, model string, messages []map[string]interface{}, timeout time.Duration) (string, error) {
	if config.APIKey == "" {
		return "", fmt.Errorf("API key not configured")
	}
	payload := map[string]interface{}{
		"model":    model,
		"messages": messages,
		"stream":   false,
	}
	useThinkingParam := config.Provider != "gemini" && supportsThinkingControl(model) && !thinkingEnabled()
	if useThinkingParam {
		payload["thinking"] = map[string]interface{}{"type": "disabled"}
	}
	if config.Provider == "openai" {
		// Gateway OpenAI-compatible: paksa output JSON valid (wrapper {"questions": [...]} ditangani extractJSONArray).
		payload["response_format"] = map[string]interface{}{"type": "json_object"}
	}

	resp, body, err := doChatRequest(config, payload, timeout)
	if err != nil {
		return "", err
	}
	// Endpoint/model versi lama bisa menolak parameter `thinking` → ulangi tanpa itu.
	if resp.StatusCode == http.StatusBadRequest && useThinkingParam {
		delete(payload, "thinking")
		resp, body, err = doChatRequest(config, payload, timeout)
		if err != nil {
			return "", err
		}
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}
	var response ChatResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}
	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	content := response.Choices[0].Message.Content
	if strings.TrimSpace(content) == "" {
		// Reasoning models can leave "content" empty and answer in reasoning_content.
		content = response.Choices[0].Message.ReasoningContent
	}
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("model returned an empty response")
	}
	return content, nil
}

// CallAI makes a request to the AI service (OpenAI-compatible chat completions).
func CallAI(config AIConfig, systemPrompt, userPrompt string) (string, error) {
	return postChatCompletion(config, config.Model, []map[string]interface{}{
		{"role": "system", "content": systemPrompt},
		{"role": "user", "content": userPrompt},
	}, 90*time.Second)
}

// CallAIMultimodal sends text + embedded images as data URLs (OpenAI-compatible content parts).
func CallAIMultimodal(config AIConfig, systemPrompt, userText string, images []string) (string, error) {
	model := pickVisionChatModel(config)
	var parts []interface{}
	if strings.TrimSpace(userText) != "" {
		parts = append(parts, map[string]string{
			"type": "text",
			"text": userText + "\n\nParse into structured JSON exactly as described in the system message. Output ONLY the JSON array.",
		})
	} else {
		parts = append(parts, map[string]string{
			"type": "text",
			"text": "Extract and parse EVERY question visible in the images into the JSON array format described in the system message. Output ONLY the JSON array.",
		})
	}
	for _, img := range images {
		parts = append(parts, map[string]interface{}{
			"type":      "image_url",
			"image_url": map[string]string{"url": img},
		})
	}
	return postChatCompletion(config, model, []map[string]interface{}{
		{"role": "system", "content": systemPrompt},
		{"role": "user", "content": parts},
	}, 90*time.Second)
}

// ParsedQuestion represents a parsed question
type ParsedQuestion struct {
	Type          string   `json:"type"`
	Title         string   `json:"title"`
	Content       string   `json:"content"`
	Options       []string `json:"options,omitempty"`
	ImageURLs     []string `json:"image_urls,omitempty"`
	CorrectAnswer string   `json:"correct_answer"`
	Explanation   string   `json:"explanation,omitempty"`
	Difficulty    string   `json:"difficulty"`
	Points        int      `json:"points"`
	Tags          []string `json:"tags"`
	// Part is the section/part title the question belongs to ("Bagian A: Pilihan Ganda").
	Part string `json:"part,omitempty"`
	// PartInstruction is the instruction line of that section, when the source has one.
	PartInstruction string `json:"part_instruction,omitempty"`
}

// ParsedPart is a section detected in the source ("Bagian A", "Soal Uraian").
type ParsedPart struct {
	Title       string `json:"title"`
	Instruction string `json:"instruction,omitempty"`
}

// ParseResult represents the parsing result
type ParseResult struct {
	Success   bool             `json:"success"`
	Questions []ParsedQuestion `json:"questions"`
	Parts     []ParsedPart     `json:"parts,omitempty"`
	Errors    []string         `json:"errors,omitempty"`
	Warnings  []string         `json:"warnings,omitempty"`
	// Engine: "ai" atau "manual" — mesin yang benar-benar menghasilkan hasil ini.
	Engine string `json:"engine,omitempty"`
}

// Mode parsing yang didukung.
const (
	ParseModeAuto   = "auto"   // AI dulu, fallback ke parser aturan
	ParseModeAI     = "ai"     // hanya AI (tetap fallback bila gagal, tapi diberi tahu)
	ParseModeManual = "manual" // hanya parser aturan sistem (tanpa panggilan AI)
)

// ParseQuestionsWithAI memproses teks/gambar menjadi soal.
//
// mode: "auto" (default, AI lalu fallback), "ai" (AI saja), "manual"
// (hanya parser aturan sistem — tanpa memanggil AI sama sekali).
func ParseQuestionsWithAI(text string, images []string, mode string) (*ParseResult, error) {
	if err := validateImageDataURLs(images); err != nil {
		return &ParseResult{Success: false, Errors: []string{err.Error()}}, nil
	}

	mode = strings.ToLower(strings.TrimSpace(mode))

	// Mode manual: lewati AI sepenuhnya.
	if mode == ParseModeManual {
		if len(images) > 0 {
			return &ParseResult{
				Success: false,
				Engine:  "manual",
				Errors: []string{
					"Parser manual tidak bisa membaca gambar. Pilih mode AI untuk memproses gambar, atau tempel teks soalnya.",
				},
			}, nil
		}
		res := ParseQuestionsSimple(text)
		res.Engine = "manual"
		return res, nil
	}

	config := GetAIConfig()

	if config.APIKey == "" {
		if len(images) > 0 {
			return &ParseResult{
				Success: false,
				Errors: []string{
					"Parsing gambar memerlukan GEMINI_API_KEY atau ZAI_API_KEY di backend (.env). Parser teks saja tetap bisa tanpa kunci.",
				},
			}, nil
		}
		res := ParseQuestionsSimple(text)
		res.Engine = "manual"
		return res, nil
	}

	text = stripMediaMarkerLines(text)

	systemPrompt := `You are an expert at parsing educational exam papers into structured JSON.
Parse the given text and/or exam images into structured JSON questions.

Rules:
1. Identify question types: multiple-choice, true-false, short-answer, essay, matching, fill-blank, step-by-step
2. Extract options for multiple-choice questions
3. Identify the correct answer
4. Assign difficulty: easy, medium, or hard
5. Assign points: easy=5, medium=8, hard=10
6. Generate relevant tags based on the subject matter
7. Create a brief LABEL title for each question (max 60 characters) — a short topic label such as "KPK dua bilangan" or "Organ pencernaan". NEVER copy the question text into the title; if you cannot think of a label, use "Soal <number>".
8. If the source is an image (screenshot, handwritten, printed paper), read all visible questions; describe figures/diagrams briefly inside "content" when needed
9. Lines like [!above URL], [!below URL], [!option-b URL], [!URL], or [!Link Image] are image placeholders only — never separate questions. Omit them from "content". Count ONLY numbered stems (1., 2., 3., …) to decide how many questions exist.
10. MATHEMATICS: Put all math expressions in LaTeX inside dollar delimiters. Inline math: $...$ (e.g. $\\sqrt{48}$, $x^2$, $\\frac{a}{b}$). Display/block equations: $$...$$. Use \\sqrt{}, ^{}, \\frac{}{}, not Unicode-only symbols (avoid bare √ or ² without LaTeX). Apply the same rule to "options", "correct_answer", and "explanation" when they contain formulas.

SECTIONS / PARTS (very important):
11. Exam papers are often split into sections. Detect every section header such as "Bagian A", "Bagian B", "Part I", "I. Pilihan Ganda", "A. Soal Uraian", "II. Isian Singkat", "## Uraian", "Pilihan Ganda", "Benar–Salah", "Menjodohkan".
12. A section header is NEVER a question: do not emit it as a question and do not include it in a question's "content".
13. For every question set "part" to the exact section title it belongs to (e.g. "Bagian A: Pilihan Ganda"). Questions before any header get "part": "" or omit it. Never invent sections that are not in the source.
14. Instructions under a header ("Pilihlah jawaban yang paling tepat!", "Berilah tanda silang (X)…") belong to the section, not to a question: leave them out of "content" and put the instruction sentence in "part_instruction" for every question of that section.

ANSWER KEYS AND METADATA:
15. Recognise all answer markers: "Correct answer:", "Answer:", "Kunci:", "Kunci Jawaban:", "Jawaban:", "Jawaban Benar:". The key may be a letter (b / B / b.) or full text.
16. Recognise explanations: "Pembahasan:", "Penjelasan:", "Explanation:".
17. Recognise Indonesian/Markdown authoring markers: **[PG-001]** = multiple-choice, **[IS-001]** = fill-blank (isian singkat), **[BS-001]** = true-false (benar-salah), **[JD-…]/[MJ-…]** = matching, **[UR-…]/[ES-…]** = essay.
18. Ignore metadata bullets that are not part of a question: "- Kelas:", "- Mapel:", "- Bab:", "- Halaman:", "- Tipe:", "- Jumlah:", "- Tingkat:", "- credit:", "- alt:". A "- Tingkat: sulit" line sets that question's difficulty (mudah=easy, sedang=medium, sulit=hard). A "- gambar: https://…" line adds that URL to the question's "image_urls".
19. For true-false questions the correct answer MUST be exactly "true" or "false" (map Benar/Betul/True -> true, Salah/False -> false).

Output ONLY valid JSON array in this exact format:
[
  {
    "type": "multiple-choice",
    "title": "Question Title",
    "part": "Bagian A: Pilihan Ganda",
    "part_instruction": "Pilihlah jawaban yang paling tepat!",
    "content": "The actual question text",
    "options": ["Option A", "Option B", "Option C", "Option D"],
    "image_urls": ["https://example.com/diagram.png"],
    "correct_answer": "Option A",
    "explanation": "Brief explanation",
    "difficulty": "easy",
    "points": 5,
    "tags": ["subject", "topic"]
  }
]
(Omit "options" for non-MCQ. Include "image_urls" when the user text has [!https://…] or other explicit image URLs for that question.)

For true/false questions remove "options" from output; correct_answer is "true" or "false".

For short-answer, essay, matching, fill-blank: omit "options" when not applicable; correct_answer is model answer or grading key.

20. STEP-BY-STEP: use type "step-by-step" for a multi-stage problem whose source lists ordered steps ("Langkah 1:", "Langkah 2:", "Step 1:", or lines written as "Langkah N: <instruksi> | Kunci: <jawaban>"). "options" holds the step instructions in order. "correct_answer" is a JSON array STRING with one answer key per step, in the same order and the same count, e.g. "[\"3x = 15\", \"x = 5\"]". Do not use step-by-step for a plain question with a single answer.`

	var response string
	var err error
	if len(images) > 0 {
		userText := strings.TrimSpace(text)
		if userText != "" {
			userText = "Context text from the user (may duplicate or supplement what is in images):\n\n" + userText
		}
		response, err = CallAIMultimodal(config, systemPrompt, userText, images)
	} else {
		userPrompt := fmt.Sprintf("Parse these questions into structured JSON:\n\n%s", text)
		response, err = CallAI(config, systemPrompt, userPrompt)
	}
	if err != nil {
		if len(images) > 0 {
			return &ParseResult{
				Success: false,
				Errors:  []string{fmt.Sprintf("VISION_API_ERROR: %v", err)},
			}, nil
		}
		fallback := ParseQuestionsSimple(text)
		fallback.Engine = "manual"
		fallback.Warnings = append(fallback.Warnings, fmt.Sprintf("AI parser tidak tersedia (%v) — memakai parser teks.", err))
		return fallback, nil
	}

	jsonStr, ok := extractJSONArray(response)
	if !ok {
		return parseFallback(text, images, "AI tidak mengembalikan JSON valid")
	}

	var questions []ParsedQuestion
	if err := json.Unmarshal([]byte(jsonStr), &questions); err != nil {
		repaired := repairJSON(jsonStr)
		if repaired == jsonStr || json.Unmarshal([]byte(repaired), &questions) != nil {
			return parseFallback(text, images, "Output AI bukan JSON yang valid")
		}
	}

	warnings := []string{}
	questions = normalizeParsedQuestions(questions, &warnings)
	if len(questions) == 0 {
		if len(images) > 0 {
			return &ParseResult{
				Success:   false,
				Questions: []ParsedQuestion{},
				Errors:    []string{"Tidak ada soal yang terbaca dari gambar. Coba gambar yang lebih jelas."},
				Warnings:  warnings,
			}, nil
		}
		return parseFallback(text, images, "Tidak ada soal yang terbaca")
	}

	assignFallbackTitles(questions)
	warnings = append(warnings, fmt.Sprintf("Parsed %d questions successfully", len(questions)))
	return &ParseResult{
		Success:   true,
		Questions: questions,
		Parts:     collectParts(questions, nil),
		Warnings:  warnings,
		Engine:    "ai",
	}, nil
}

// parseFallback keeps text-only parsing useful when the AI returns junk, and
// surfaces a clear error when the input was images (no text to fall back to).
func parseFallback(text string, images []string, reason string) (*ParseResult, error) {
	if len(images) > 0 {
		return &ParseResult{
			Success: false,
			Errors:  []string{reason + ". Coba lagi atau perkecil/ganti gambar."},
		}, nil
	}
	fallback := ParseQuestionsSimple(text)
	fallback.Engine = "manual"
	fallback.Warnings = append(fallback.Warnings, reason+" — memakai parser teks.")
	return fallback, nil
}

// extractJSONArray finds the outermost JSON array in an LLM response, tolerating
// markdown code fences and {"questions": [...]} wrappers. The scan is string-aware
// so brackets inside strings do not truncate the payload.
func extractJSONArray(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, "```"); i >= 0 {
		rest := s[i+3:]
		rest = strings.TrimPrefix(rest, "json")
		rest = strings.TrimPrefix(rest, "JSON")
		rest = strings.TrimLeft(rest, "\r\n\t ")
		if j := strings.Index(rest, "```"); j >= 0 {
			s = rest[:j]
		} else {
			s = rest
		}
	}

	start := strings.Index(s, "[")
	if start < 0 {
		if i := strings.Index(strings.ToLower(s), "\"questions\""); i >= 0 {
			if j := strings.Index(s[i:], "["); j >= 0 {
				start = i + j
			}
		}
	}
	if start < 0 {
		return "", false
	}

	depth := 0
	inStr := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return s[start : i+1], true
			}
		}
	}
	return "", false
}

// repairJSON cleans up the most common LLM JSON mistakes so one malformed
// trailing comma does not throw away an entire parse.
func repairJSON(s string) string {
	out := strings.NewReplacer(
		"\u201c", "\"", "\u201d", "\"", "\u2018", "'", "\u2019", "'",
		": True", ": true", ": False", ": false", ": None", ": null",
		":True", ":true", ":False", ":false", ":None", ":null",
	).Replace(s)

	// Drop trailing commas before ] or }.
	var b strings.Builder
	b.Grow(len(out))
	for i := 0; i < len(out); i++ {
		c := out[i]
		if c == ',' {
			j := i + 1
			for j < len(out) && (out[j] == ' ' || out[j] == '\n' || out[j] == '\r' || out[j] == '\t') {
				j++
			}
			if j < len(out) && (out[j] == ']' || out[j] == '}') {
				continue
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}

// isMediaMarkerLine detects image placeholder lines that must not become question stems.
func isMediaMarkerLine(line string) bool {
	s := strings.TrimSpace(line)
	if s == "" {
		return false
	}
	patterns := []string{
		`(?i)^\[!\s*link\s*image\s*\]$`,
		`(?i)^\[link\s*image\]$`,
		`(?i)^\[!\s*link\s*image\s*\]\s*:\s*https?://`,
		`(?i)^\[!\s*(above|below|after)\s*:?\s*https?://`,
		`(?i)^\[!\s*(above|below|after)\s+https?://`,
		`(?i)^\[!\s*option\s*[-_\s]*[a-d]\s*:?\s*https?://`,
		`(?i)^\[!\s*option\s*[-_\s]*[a-d]\s+https?://`,
		`(?i)^\[!\s*https?://[^\]]+\]\s*$`,
		`(?i)^https?://\S+\.(png|jpe?g|gif|webp)(\?[^\s]*)?\s*$`,
	}
	for _, p := range patterns {
		if matched, _ := regexp.MatchString(p, s); matched {
			return true
		}
	}
	return false
}

// stripMediaMarkerLines removes standalone image marker lines (AI / simple parser input).
func stripMediaMarkerLines(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if !isMediaMarkerLine(line) {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func appendUniqueURL(urls []string, url string) []string {
	url = strings.TrimSpace(url)
	if url == "" {
		return urls
	}
	for _, u := range urls {
		if u == url {
			return urls
		}
	}
	return append(urls, url)
}

// ensureFirstQuestionNumbered prepends "1. " when a user pastes a bare stem with
// no numbering, so the line-based parser can open a question. It leaves text that
// already has stems (numbered or tagged) untouched.
func ensureFirstQuestionNumbered(text string) string {
	lines := strings.Split(text, "\n")
	seenContent := false
	for i, line := range lines {
		s := strings.TrimSpace(line)
		if s == "" || isMediaMarkerLine(s) || isDividerLine(s) {
			continue
		}
		clean := strings.TrimSpace(cleanMarkdownLine(s))
		if strings.HasPrefix(clean, "#") || reOtherMeta.MatchString(clean) {
			continue
		}
		if _, _, ok := matchStem(s); ok {
			return text
		}
		if seenContent {
			continue
		}
		seenContent = true
		if looksLikeStemLine(clean) {
			lines[i] = "1. " + clean
			return strings.Join(lines, "\n")
		}
		break
	}
	return text
}

// ---------------------------------------------------------------------------
// Rule-based (no AI) parser — supports both the numbered exam format
// ("1. stem / a) option / Correct answer: b") and the Markdown authoring format
// ("**[PG-001]** stem / A. option / - Kunci: B / - Pembahasan: …"), including
// "Bagian A" style sections.
// ---------------------------------------------------------------------------

var (
	reStemNumbered = regexp.MustCompile(`^\(?(\d{1,3})\)?[.)]\s*(.+)$`)
	reStemSoal     = regexp.MustCompile(`(?i)^soal\s+(\d{1,3})\s*[:.)]?\s*(.+)$`)
	reStemTagged   = regexp.MustCompile(`^\**\s*[\[(]([^\]\)\n]{1,60})[\]\)]\**\s*(.*)$`)
	reOptionLetter = regexp.MustCompile(`^\**\s*[-•*]?\s*([a-eA-E])\s*[.)]\s*\**\s*(.+)$`)
	reAnswerLine   = regexp.MustCompile(`(?i)^\**\s*[-•*]?\s*(kunci\s*jawaban|kunci|jawaban\s*benar|jawaban|correct\s*answer|answer|ans)\s*[:=]\s*(.+?)\s*\**\s*$`)
	reExplainLine  = regexp.MustCompile(`(?i)^\**\s*[-•*]?\s*(pembahasan|penjelasan|explanation|alasan)\s*[:=]\s*(.+?)\s*\**\s*$`)
	reStepLine     = regexp.MustCompile(`(?i)^\**\s*(langkah|step)\s*\d*\s*[:.)]\s*(.+?)\s*\|\s*(kunci|jawaban)\s*[:=]\s*(.+?)\s*\**\s*$`)
	reImageMeta    = regexp.MustCompile(`(?i)^\**\s*[-•*]?\s*(gambar|image|img|foto)\s*[:=]\s*(.+?)\s*\**\s*$`)
	reDiffMeta     = regexp.MustCompile(`(?i)^\**\s*[-•*]?\s*(tingkat|kesulitan|difficulty)\s*[:=]\s*(.+?)\s*\**\s*$`)
	reOtherMeta    = regexp.MustCompile(`(?i)^\**\s*[-•*]?\s*(kelas|mapel|mata\s*pelajaran|bab|halaman|hal|tipe|jenis|jumlah|dibuat|sumber|versi|credit|kredit|alt|penulis|author)\s*[:=]`)
	reRomanSection = regexp.MustCompile(`^([IVXLC]{1,7})\s*[.)]\s*(.*)$`)
	reUpperSection = regexp.MustCompile(`^([A-Z])\s*[.)]\s+(.+)$`)
	reSectionHead  = regexp.MustCompile(`(?i)^(bagian|part|section)\b`)
	reSectionOnly  = regexp.MustCompile(`(?i)^(soal\s+)?(pilihan\s+ganda|pg|uraian|isian(\s+singkat)?|essay|esai|menjodohkan|memasangkan|benar\s*[-–/]\s*salah|jawaban\s+singkat|soal\s+cerita)\s*[:!.]?$`)
	reSectionWord  = regexp.MustCompile(`(?i)(pilihan\s+ganda|uraian|isian|essay|esai|menjodohkan|memasangkan|benar\s*[-–/]?\s*salah|jawaban\s+singkat)`)
	reInstruction  = regexp.MustCompile(`(?i)^\**\s*(berilah|beri|pilihlah|pilih|jawablah|jawab|tulislah|tulis|isilah|isi|pasangkan|jodohkan|kerjakan|centang|perhatikan|bacalah|baca)\b`)
	reURLInLine    = regexp.MustCompile(`https?://[^\s\])"'<>]+`)
	reDividerLine  = regexp.MustCompile(`^([-*_=#~]{3,}|<hr\s*/?>)$`)
	reTagKey       = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,11}[-_][A-Za-z0-9]+$`)
)

// extractAfterAnswerKeywords returns text after "correct answer:" / "answer:" (case-insensitive).
func extractAfterAnswerKeywords(line string) string {
	s := strings.TrimSpace(line)
	low := strings.ToLower(s)
	for _, key := range []string{"correct answer:", "kunci jawaban:", "kunci:", "jawaban benar:", "jawaban:", "answer:", "ans:"} {
		if idx := strings.Index(low, key); idx >= 0 {
			return strings.TrimSpace(s[idx+len(key):])
		}
	}
	return s
}

// cleanMarkdownLine strips markdown decoration that wraps a whole line.
func cleanMarkdownLine(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimLeft(s, "#> \t")
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "*_`")
	return strings.TrimSpace(s)
}

// invisibleChars maps no-break spaces to plain spaces and drops zero-width
// characters; text copied from ChatGPT/Docs carries them and they defeat the
// line regexes (RE2's \s only matches ASCII whitespace).
var invisibleChars = strings.NewReplacer(
	"\u00a0", " ", "\u202f", " ",
	"\u200b", "", "\u200c", "", "\u200d", "", "\ufeff", "",
)

// cleanStepLine strips markdown and bullet decoration ("> ", "### ", "- ",
// "**Langkah 1:**", backticks) so reStepLine also matches decorated step lines.
func cleanStepLine(s string) string {
	s = strings.NewReplacer("**", "", "__", "", "`", "").Replace(s)
	s = strings.TrimLeft(strings.TrimSpace(s), "#>-•*_ \t")
	return strings.TrimRight(s, "*_ \t")
}

func isDividerLine(line string) bool {
	s := strings.TrimSpace(line)
	if s == "" {
		if line == "" {
			return true
		}
		return true
	}
	return reDividerLine.MatchString(cleanMarkdownLine(s))
}

func firstURL(s string) string {
	m := reURLInLine.FindString(s)
	return strings.TrimRight(m, ").,;:'\"")
}

// tagToken returns the identifier part of an authoring tag: "PG-001, mudah,
// bergambar mulut" -> "PG-001".
func tagToken(tag string) string {
	t := strings.TrimSpace(tag)
	if i := strings.IndexAny(t, ",;|"); i >= 0 {
		t = t[:i]
	}
	t = strings.TrimSpace(t)
	fields := strings.Fields(t)
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimSpace(fields[0])
}

// isQuestionTag reports whether a bracketed tag is a question identifier
// (PG-001, IS-003, BS-02, MC-1 …) rather than a stray "[…]" in prose.
func isQuestionTag(tag string) bool {
	up := strings.ToUpper(tagToken(tag))
	if up == "" {
		return false
	}
	for _, p := range []string{"PG", "IS", "BS", "JD", "MJ", "UR", "ES", "TF", "MC", "SA", "FILL", "SHORT", "ESSAY", "MATCH", "SOAL", "PGAMBAR", "PG-GAMBAR"} {
		if up == p || strings.HasPrefix(up, p+"-") || strings.HasPrefix(up, p+"_") {
			return true
		}
	}
	return reTagKey.MatchString(up)
}

// typeFromTag maps an authoring tag such as PG-001 / IS-02 / BS-3 to a question type.
func typeFromTag(tag string) string {
	up := strings.ToUpper(tagToken(tag))
	up = strings.TrimSuffix(up, "-GAMBAR")
	switch {
	case strings.HasPrefix(up, "PG"), strings.HasPrefix(up, "MC"):
		return "multiple-choice"
	case strings.HasPrefix(up, "BS"), strings.HasPrefix(up, "TF"):
		return "true-false"
	case strings.HasPrefix(up, "IS"), strings.HasPrefix(up, "FILL"):
		return "fill-blank"
	case strings.HasPrefix(up, "JD"), strings.HasPrefix(up, "MJ"), strings.HasPrefix(up, "MATCH"):
		return "matching"
	case strings.HasPrefix(up, "UR"), strings.HasPrefix(up, "ES"):
		return "essay"
	case strings.HasPrefix(up, "SA"), strings.HasPrefix(up, "SHORT"):
		return "short-answer"
	}
	return ""
}

// normalizeQuestionTypeLoose maps loose/AI/localised type names to the DB values.
func normalizeQuestionTypeLoose(t string) string {
	s := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(t, "_", "-")))
	switch s {
	case "mcq", "multiple choice", "multiplechoice", "multi-choice", "pg", "pilihan ganda":
		return "multiple-choice"
	case "tf", "true/false", "boolean", "benar-salah", "benar/salah", "bs":
		return "true-false"
	case "shortanswer", "short answer", "short", "jawaban singkat":
		return "short-answer"
	case "fillblank", "fill blank", "fill-in-the-blank", "fill in the blank", "isian", "isian singkat":
		return "fill-blank"
	case "esai", "uraian":
		return "essay"
	case "menjodohkan", "memasangkan", "match":
		return "matching"
	case "step-by-step", "step by step", "stepbystep", "bertahap", "langkah":
		return "step-by-step"
	}
	switch s {
	case "multiple-choice", "true-false", "short-answer", "essay", "matching", "fill-blank", "step-by-step":
		return s
	}
	return ""
}

func normalizeDifficultyLoose(d string) string {
	switch strings.ToLower(strings.TrimSpace(d)) {
	case "easy", "mudah", "gampang", "rendah":
		return "easy"
	case "hard", "sulit", "sukar", "tinggi":
		return "hard"
	case "medium", "sedang", "menengah", "normal":
		return "medium"
	}
	return "medium"
}

func defaultPointsFor(difficulty string) int {
	switch difficulty {
	case "easy":
		return 5
	case "hard":
		return 10
	default:
		return 8
	}
}

// trueFalseValue maps localised true/false answers to the "true"/"false" the
// grader expects (see present.AnswersMatch).
func trueFalseValue(s string) (string, bool) {
	v := strings.ToLower(strings.TrimSpace(s))
	v = strings.Trim(v, "*_`'\"()[]{}")
	v = strings.Trim(v, ".:;!,")
	v = strings.TrimSpace(v)
	switch v {
	case "true", "t", "benar", "b", "ya", "yes", "y", "betul", "correct":
		return "true", true
	case "false", "f", "salah", "s", "tidak", "no", "n", "wrong", "incorrect":
		return "false", true
	}
	return "", false
}

// resolveMCQAnswer maps a letter key ("b", "(B)", "B.") to the option text.
func resolveMCQAnswer(answer string, options []string) string {
	a := strings.TrimSpace(answer)
	if a == "" || len(options) == 0 {
		return a
	}
	low := strings.ToLower(a)
	low = strings.Trim(low, "*_`'\" ")
	low = strings.Trim(low, "()[]{}")
	low = strings.Trim(low, ".:;) ")
	low = strings.TrimSpace(low)
	for _, prefix := range []string{"option ", "pilihan ", "jawaban "} {
		low = strings.TrimSpace(strings.TrimPrefix(low, prefix))
	}
	if len(low) == 1 && low[0] >= 'a' && low[0] <= 'z' {
		idx := int(low[0] - 'a')
		if idx >= 0 && idx < len(options) {
			return options[idx]
		}
	}
	return a
}

// assignFallbackTitles memberi nomor urut yang benar pada seluruh daftar
// (normalizeParsedQuestions memproses satu per satu di parser manual, sehingga
// indeksnya selalu 1 dan semua judul akan menjadi "Soal 1").
func assignFallbackTitles(qs []ParsedQuestion) {
	for i := range qs {
		if strings.TrimSpace(qs[i].Title) == "" || titleRepeatsContent(qs[i].Title, qs[i].Content) {
			qs[i].Title = fallbackTitle(i + 1)
		}
	}
}

// titleRepeatsContent melaporkan judul yang sekadar menyalin (awal) isi soal.
func titleRepeatsContent(title, content string) bool {
	norm := func(s string) string {
		s = strings.ReplaceAll(s, "\n", " ")
		s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "…"))
		return strings.ToLower(strings.Join(strings.Fields(s), " "))
	}
	t, c := norm(title), norm(content)
	if t == "" || c == "" {
		return false
	}
	if t == c {
		return true
	}
	return len([]rune(t)) >= 20 && strings.HasPrefix(c, t)
}

// truncateText trims s to at most max runes (UTF-8 safe).
func truncateText(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return strings.TrimSpace(string(r[:max]))
}

// fallbackTitle memberi label singkat ketika sumber tidak menyediakan judul.
//
// Penting: JANGAN memakai potongan teks soal sebagai judul. UI menampilkan
// judul dan isi secara terpisah, sehingga judul yang menyalin isi membuat soal
// tampak ganda (satu tebal, satu kecil dengan tulisan sama).
func fallbackTitle(n int) string {
	return fmt.Sprintf("Soal %d", n)
}

func inferTypeFromContent(content string, optionCount int) string {
	if optionCount >= 2 {
		return "multiple-choice"
	}
	low := strings.ToLower(content)
	switch {
	case strings.Contains(low, "true or false"), strings.Contains(low, "benar atau salah"),
		strings.Contains(low, "benar-salah"), strings.Contains(low, "benar/salah"):
		return "true-false"
	case strings.Contains(low, "jelaskan"), strings.Contains(low, "uraikan"), strings.Contains(low, "sebutkan"),
		strings.Contains(low, "tuliskan pendapat"), strings.Contains(low, "essay"), strings.Contains(low, "uraian"):
		return "essay"
	case strings.Contains(low, "....") || strings.Contains(low, "…"):
		return "fill-blank"
	}
	return "short-answer"
}

// normalizeParsedQuestions repairs parser/AI output so it can be saved to the
// question bank without tripping validation (MCQ needs ≥2 options, difficulty
// and points must be set, true/false keys must be true|false).
func normalizeParsedQuestions(qs []ParsedQuestion, warnings *[]string) []ParsedQuestion {
	out := make([]ParsedQuestion, 0, len(qs))
	for i := range qs {
		q := qs[i]
		q.Content = strings.TrimSpace(stripMediaMarkerLines(q.Content))
		q.Explanation = strings.TrimSpace(q.Explanation)
		q.Part = strings.TrimSpace(q.Part)
		q.CorrectAnswer = strings.TrimSpace(q.CorrectAnswer)
		if q.Content == "" {
			continue
		}

		opts := make([]string, 0, len(q.Options))
		for _, o := range q.Options {
			o = strings.TrimSpace(stripMediaMarkerLines(o))
			if o != "" {
				opts = append(opts, o)
			}
		}
		q.Options = opts

		typ := normalizeQuestionTypeLoose(q.Type)
		if typ == "" {
			typ = inferTypeFromContent(q.Content, len(q.Options))
		}

		if typ == "multiple-choice" && len(q.Options) < 2 {
			q.Options = nil
			if _, isTF := trueFalseValue(q.CorrectAnswer); isTF && strings.Contains(strings.ToLower(q.Content), "benar") {
				typ = "true-false"
			} else {
				typ = inferTypeFromContent(q.Content, 0)
			}
			if warnings != nil {
				*warnings = append(*warnings, fmt.Sprintf("Soal %d tidak punya opsi jawaban — diubah menjadi %s.", i+1, typ))
			}
		}

		if typ == "true-false" {
			q.Options = nil
			if v, ok := trueFalseValue(q.CorrectAnswer); ok {
				q.CorrectAnswer = v
			}
		} else if typ == "multiple-choice" {
			q.CorrectAnswer = resolveMCQAnswer(q.CorrectAnswer, q.Options)
		}

		q.Type = typ
		q.Difficulty = normalizeDifficultyLoose(q.Difficulty)
		if q.Points <= 0 {
			q.Points = defaultPointsFor(q.Difficulty)
		}
		// Judul yang menyalin isi soal dibuang di sini; penomoran "Soal N"
		// dilakukan sekali untuk seluruh daftar oleh assignFallbackTitles.
		q.Title = strings.TrimSpace(q.Title)
		if q.Title != "" && titleRepeatsContent(q.Title, q.Content) {
			q.Title = ""
		}
		q.Title = truncateText(q.Title, 200)
		if q.Tags == nil {
			q.Tags = []string{}
		}
		out = append(out, q)
	}
	return out
}

// collectParts returns every distinct part referenced by the questions (and any
// explicitly declared part), in first-seen order.
func collectParts(qs []ParsedQuestion, declared []ParsedPart) []ParsedPart {
	out := []ParsedPart{}
	index := map[string]int{}
	add := func(title string) int {
		title = strings.TrimSpace(title)
		if title == "" {
			return -1
		}
		if i, ok := index[title]; ok {
			return i
		}
		out = append(out, ParsedPart{Title: title})
		index[title] = len(out) - 1
		return len(out) - 1
	}
	for _, d := range declared {
		if i := add(d.Title); i >= 0 && d.Instruction != "" && out[i].Instruction == "" {
			out[i].Instruction = d.Instruction
		}
	}
	for _, q := range qs {
		if i := add(q.Part); i >= 0 && q.PartInstruction != "" && out[i].Instruction == "" {
			out[i].Instruction = strings.TrimSpace(q.PartInstruction)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// detectSectionTitle returns the section title when the line is a section header.
// allowRoman permits a bare roman-numeral line (e.g. "I. Berilah tanda silang …")
// to be treated as a header when no question has started yet.
func detectSectionTitle(line string, allowRoman bool) string {
	clean := cleanMarkdownLine(line)
	if clean == "" || len([]rune(clean)) > 120 {
		return ""
	}
	if reSectionHead.MatchString(clean) {
		return clean
	}
	if m := reRomanSection.FindStringSubmatch(clean); m != nil {
		rest := strings.TrimSpace(m[2])
		if reSectionWord.MatchString(rest) || (allowRoman && rest != "") {
			return clean
		}
	}
	if m := reUpperSection.FindStringSubmatch(clean); m != nil {
		rest := strings.TrimSpace(m[2])
		if reSectionWord.MatchString(rest) {
			return clean
		}
	}
	if len([]rune(clean)) <= 40 && reSectionOnly.MatchString(clean) {
		return clean
	}
	return ""
}

// reInlineOption finds option markers inside a single line, e.g.
// "- A. 1512 — B. 1521 — C. 1522 — D. 1596".
var reInlineOption = regexp.MustCompile(`(?:^|[\s—–\-|])([A-Ea-e])\s*[.)]\s+`)

// splitInlineOptions splits a compact one-line option list into its options.
// It requires at least two markers with sequential letters starting at A/a.
func splitInlineOptions(line string) []string {
	clean := strings.TrimSpace(line)
	clean = strings.TrimPrefix(clean, "- ")
	clean = strings.TrimSpace(clean)
	locs := reInlineOption.FindAllStringSubmatchIndex(clean, -1)
	if len(locs) < 2 {
		return nil
	}
	out := make([]string, 0, len(locs))
	for i, loc := range locs {
		letter := strings.ToLower(clean[loc[2]:loc[3]])
		if int(letter[0]-'a') != i {
			return nil
		}
		textStart := loc[1]
		textEnd := len(clean)
		if i+1 < len(locs) {
			textEnd = locs[i+1][0]
		}
		text := strings.TrimSpace(clean[textStart:textEnd])
		text = strings.TrimRight(text, " —–-|")
		text = strings.TrimSpace(text)
		if text == "" {
			return nil
		}
		out = append(out, text)
	}
	return out
}

// stemMetaWords mark a leading "(…)" group as question metadata rather than
// part of the stem: "**[PG-021]** (sedang, bergambar: `x.jpg`) Perhatikan …".
var stemMetaRe = regexp.MustCompile(`(?i)^(mudah|sedang|sulit|hots|bergambar|gambar|image|pg|is|bs|jpg|jpeg|png|webp|\.jpg|\.png|\d+\s*poin|poin)`)

// stripStemMetaPrefix removes leading parenthesised metadata groups.
func stripStemMetaPrefix(rest string) string {
	for {
		s := strings.TrimSpace(rest)
		if !strings.HasPrefix(s, "(") {
			return s
		}
		end := strings.Index(s, ")")
		if end < 0 || end > 120 {
			return s
		}
		inner := strings.TrimSpace(s[1:end])
		first := inner
		if i := strings.IndexAny(first, ",;:"); i >= 0 {
			first = first[:i]
		}
		first = strings.TrimSpace(first)
		if first == "" || !stemMetaRe.MatchString(first) {
			return s
		}
		rest = s[end+1:]
	}
}

// splitAnswerAndExplanation splits "D — Pembahasan: …" into key and explanation.
func splitAnswerAndExplanation(value string) (string, string) {
	low := strings.ToLower(value)
	for _, key := range []string{"pembahasan:", "penjelasan:", "explanation:"} {
		idx := strings.Index(low, key)
		if idx < 0 {
			continue
		}
		answer := strings.TrimSpace(value[:idx])
		answer = strings.TrimRight(answer, "—–-|: ")
		expl := strings.TrimSpace(value[idx+len(key):])
		if answer == "" {
			return value, ""
		}
		return strings.TrimSpace(answer), expl
	}
	return value, ""
}

// matchStem recognises a new question stem and returns its text plus authoring tag.
func matchStem(line string) (string, string, bool) {
	clean := strings.TrimSpace(line)
	if m := reStemNumbered.FindStringSubmatch(clean); m != nil {
		rest := strings.TrimSpace(m[2])
		if rest == "" {
			return "", "", false
		}
		return rest, "", true
	}
	if m := reStemSoal.FindStringSubmatch(clean); m != nil {
		rest := strings.TrimSpace(m[2])
		if rest == "" {
			return "", "", false
		}
		return rest, "", true
	}
	if m := reStemTagged.FindStringSubmatch(clean); m != nil && isQuestionTag(m[1]) {
		rest := stripStemMetaPrefix(strings.TrimSpace(m[2]))
		if rest == "" {
			return "", "", false
		}
		return rest, tagToken(m[1]), true
	}
	return "", "", false
}

// matchOption recognises an answer option (a) / A. / - b) and enforces the
// a→b→c→d sequence so prose lines are not mistaken for options.
func matchOption(line string, current *ParsedQuestion, style *string, count int) (string, bool) {
	if current == nil {
		return "", false
	}
	m := reOptionLetter.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	letter := strings.ToLower(m[1])
	text := strings.TrimSpace(m[2])
	if text == "" {
		return "", false
	}
	idx := int(letter[0] - 'a')
	if *style == "" {
		if count > 0 {
			return "", false
		}
		*style = "letter"
	} else if *style != "letter" {
		return "", false
	}
	if idx != count {
		return "", false
	}
	return text, true
}

// looksLikeStemLine is used to decide whether a bare first line needs a "1." prefix.
func looksLikeStemLine(line string) bool {
	clean := strings.TrimSpace(cleanMarkdownLine(line))
	if clean == "" {
		return false
	}
	if reOtherMeta.MatchString(clean) || reAnswerLine.MatchString(clean) || reExplainLine.MatchString(clean) {
		return false
	}
	if strings.HasSuffix(clean, "....") || strings.HasSuffix(clean, "…") || strings.HasSuffix(clean, "...") {
		return true
	}
	if strings.Contains(clean, "?") {
		return true
	}
	low := strings.ToLower(clean)
	return strings.Contains(low, "adalah") || strings.Contains(low, "berikut") || strings.Contains(low, "tentukan")
}

// ParseQuestionsSimple provides a rule-based parser used when AI parsing is
// unavailable or returns unusable output.
func ParseQuestionsSimple(text string) *ParseResult {
	if strings.TrimSpace(text) == "" {
		return &ParseResult{
			Success:   false,
			Questions: []ParsedQuestion{},
			Errors:    []string{"Tidak ada teks untuk diparse."},
		}
	}

	text = invisibleChars.Replace(text)
	text = ensureFirstQuestionNumbered(text)
	lines := strings.Split(text, "\n")

	var questions []ParsedQuestion
	var declaredParts []ParsedPart
	var warnings []string
	var current *ParsedQuestion
	var options []string
	var pendingImages []string
	currentPart := ""
	optionStyle := ""
	var steps, stepKeys []string

	flush := func() {
		if current == nil {
			return
		}
		q := *current
		if len(options) > 0 {
			q.Options = append([]string{}, options...)
		}
		if len(steps) > 0 {
			// Kunci per langkah = JSON array string, sama dengan format AI.
			keysJSON, _ := json.Marshal(stepKeys)
			q.Type = "step-by-step"
			q.Options = append([]string{}, steps...)
			q.CorrectAnswer = string(keysJSON)
		}
		for _, u := range pendingImages {
			q.ImageURLs = appendUniqueURL(q.ImageURLs, u)
		}
		pendingImages = nil
		normalized := normalizeParsedQuestions([]ParsedQuestion{q}, &warnings)
		if len(normalized) > 0 {
			questions = append(questions, normalized[0])
		}
		current = nil
		options = nil
		optionStyle = ""
		steps, stepKeys = nil, nil
	}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || isDividerLine(line) {
			continue
		}

		if isMediaMarkerLine(line) {
			if u := firstURL(line); u != "" {
				pendingImages = appendUniqueURL(pendingImages, u)
			}
			continue
		}

		if m := reAnswerLine.FindStringSubmatch(line); m != nil {
			if current != nil {
				answer, expl := splitAnswerAndExplanation(strings.TrimSpace(m[2]))
				current.CorrectAnswer = answer
				if expl != "" && current.Explanation == "" {
					current.Explanation = expl
				}
			}
			continue
		}
		if m := reExplainLine.FindStringSubmatch(line); m != nil {
			if current != nil {
				current.Explanation = strings.TrimSpace(m[2])
			}
			continue
		}
		if m := reImageMeta.FindStringSubmatch(line); m != nil {
			if u := firstURL(m[2]); u != "" {
				if current != nil {
					current.ImageURLs = appendUniqueURL(current.ImageURLs, u)
				} else {
					pendingImages = appendUniqueURL(pendingImages, u)
				}
			}
			continue
		}
		if m := reDiffMeta.FindStringSubmatch(line); m != nil {
			if current != nil {
				current.Difficulty = normalizeDifficultyLoose(m[2])
			}
			continue
		}
		if reOtherMeta.MatchString(line) {
			continue
		}

		if title := detectSectionTitle(line, current == nil); title != "" {
			flush()
			if title != currentPart {
				currentPart = title
				declaredParts = append(declaredParts, ParsedPart{Title: title})
			}
			continue
		}

		if stem, tag, ok := matchStem(line); ok {
			carry := pendingImages
			pendingImages = nil
			flush()
			current = &ParsedQuestion{
				Type:       typeFromTag(tag),
				Content:    stem,
				Difficulty: "medium",
				Tags:       []string{},
			}
			for _, u := range carry {
				current.ImageURLs = appendUniqueURL(current.ImageURLs, u)
			}
			if currentPart != "" {
				current.Part = currentPart
			}
			continue
		}

		if current == nil {
			// Instruction line that belongs to the current section.
			if currentPart != "" && reInstruction.MatchString(line) && len(declaredParts) > 0 {
				i := len(declaredParts) - 1
				if declaredParts[i].Instruction == "" {
					declaredParts[i].Instruction = strings.TrimSpace(cleanMarkdownLine(line))
				}
			}
			continue
		}

		if m := reStepLine.FindStringSubmatch(cleanStepLine(line)); m != nil {
			steps = append(steps, strings.TrimSpace(m[2]))
			stepKeys = append(stepKeys, strings.TrimSpace(m[4]))
			continue
		}

		if len(options) == 0 {
			if inline := splitInlineOptions(line); inline != nil {
				options = append(options, inline...)
				optionStyle = "letter"
				continue
			}
		}

		if opt, ok := matchOption(line, current, &optionStyle, len(options)); ok {
			options = append(options, opt)
			continue
		}

		// Continuation of the current stem (multi-line questions).
		extra := strings.TrimSpace(cleanMarkdownLine(line))
		if extra == "" {
			continue
		}
		if current.Content == "" {
			current.Content = extra
		} else {
			current.Content += "\n" + extra
		}
	}
	flush()

	if len(questions) == 0 {
		return &ParseResult{
			Success:   false,
			Questions: []ParsedQuestion{},
			Errors:    []string{"Tidak ada soal yang bisa diparse dari teks. Pastikan setiap soal diawali nomor (1.) atau penanda seperti [PG-001]."},
			Warnings:  warnings,
			Engine:    "manual",
		}
	}

	assignFallbackTitles(questions)

	return &ParseResult{
		Success:   true,
		Questions: questions,
		Parts:     collectParts(questions, declaredParts),
		Warnings:  warnings,
		Engine:    "manual",
	}
}
