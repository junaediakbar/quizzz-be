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
			model = "glm-3-turbo"
		}
		return AIConfig{
			BaseURL:  strings.TrimSuffix(baseURL, "/"),
			APIKey:   zaiKey,
			Model:    model,
			Provider: "zhipu",
		}
	}
}

// generateZhipuToken generates a JWT token for Zhipu AI API authentication
func generateZhipuToken(apiKey string, expSeconds int) (string, error) {
	parts := strings.Split(apiKey, ".")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid API key format")
	}

	apiID := parts[0]
	apiSecret := parts[1]

	now := time.Now()
	exp := now.Add(time.Duration(expSeconds) * time.Second)

	// Create custom header with sign_type
	type ZhipuHeader struct {
		Alg      string `json:"alg"`
		SignType string `json:"sign_type"`
	}

	// Create JWT claims with required fields for Zhipu AI
	type ZhipuClaims struct {
		ApiKey    string `json:"api_key"`
		Exp       int64  `json:"exp"`
		Timestamp int64  `json:"timestamp"`
	}

	// Create token with custom header
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"api_key":   apiID,
		"exp":       exp.Unix(),
		"timestamp": now.Unix(),
	})

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
	if config.Provider == "gemini" {
		return config.Model
	}
	return "glm-4-flash"
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
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}
	url := config.BaseURL + "/chat/completions"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	switch strings.ToLower(config.Provider) {
	case "gemini":
		req.Header.Set("Authorization", "Bearer "+config.APIKey)
	default:
		token, err := generateZhipuToken(config.APIKey, 3600)
		if err != nil {
			return "", fmt.Errorf("failed to generate token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}
	var response ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}
	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return response.Choices[0].Message.Content, nil
}

// CallAI makes a request to the AI service (OpenAI-compatible chat completions).
func CallAI(config AIConfig, systemPrompt, userPrompt string) (string, error) {
	return postChatCompletion(config, config.Model, []map[string]interface{}{
		{"role": "system", "content": systemPrompt},
		{"role": "user", "content": userPrompt},
	}, 75*time.Second)
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
	}, 120*time.Second)
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
}

// ParseResult represents the parsing result
type ParseResult struct {
	Success   bool             `json:"success"`
	Questions []ParsedQuestion `json:"questions"`
	Errors    []string         `json:"errors,omitempty"`
	Warnings  []string         `json:"warnings,omitempty"`
}

// ParseQuestionsWithAI uses AI to parse text and/or images into questions.
func ParseQuestionsWithAI(text string, images []string) (*ParseResult, error) {
	if err := validateImageDataURLs(images); err != nil {
		return &ParseResult{Success: false, Errors: []string{err.Error()}}, nil
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
		return ParseQuestionsSimple(text), nil
	}

	text = stripMediaMarkerLines(text)

	systemPrompt := `You are an expert at parsing educational questions.
Parse the given text and/or exam images into structured JSON questions.

Rules:
1. Identify question types: multiple-choice, true-false, short-answer, essay, matching, fill-blank
2. Extract options for multiple-choice questions
3. Identify the correct answer
4. Assign difficulty: easy, medium, or hard
5. Assign points: easy=3-5, medium=6-10, hard=8-15
6. Generate relevant tags based on the subject matter
7. Create a brief title for each question
8. If the source is an image (screenshot, handwritten, printed paper), read all visible questions; describe figures/diagrams briefly inside "content" when needed
9. Lines like [!above URL], [!below URL], [!option-b URL], [!URL], or [!Link Image] are image placeholders only — never separate questions. Omit them from "content". Count ONLY numbered stems (1., 2., 3., …) to decide how many questions exist.
10. MATHEMATICS: Put all math expressions in LaTeX inside dollar delimiters. Inline math: $...$ (e.g. $\\sqrt{48}$, $x^2$, $\\frac{a}{b}$). Display/block equations: $$...$$. Use \\sqrt{}, ^{}, \\frac{}{}, not Unicode-only symbols (avoid bare √ or ² without LaTeX). Apply the same rule to "options", "correct_answer", and "explanation" when they contain formulas.

Output ONLY valid JSON array in this exact format:
[
  {
    "type": "multiple-choice",
    "title": "Question Title",
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

For short-answer, essay, matching, fill-blank: omit "options" when not applicable; correct_answer is model answer or grading key.`

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
				Errors: []string{fmt.Sprintf("VISION_API_ERROR: %v", err)},
			}, nil
		}
		return ParseQuestionsSimple(text), nil
	}

	// Extract JSON from response
	// Sometimes AI adds markdown code blocks
	jsonStr := response
	if strings.Contains(jsonStr, "```json") {
		re := regexp.MustCompile("```json\\n?([\\s\\S]*?)\\n?```")
		matches := re.FindStringSubmatch(jsonStr)
		if len(matches) > 1 {
			jsonStr = matches[1]
		}
	} else if strings.Contains(jsonStr, "```") {
		re := regexp.MustCompile("```\\n?([\\s\\S]*?)\\n?```")
		matches := re.FindStringSubmatch(jsonStr)
		if len(matches) > 1 {
			jsonStr = matches[1]
		}
	}

	// Find JSON array in response
	if !strings.HasPrefix(jsonStr, "[") {
		re := regexp.MustCompile("\\[[\\s\\S]*\\]")
		matches := re.FindString(jsonStr)
		if matches != "" {
			jsonStr = matches
		}
	}

	var questions []ParsedQuestion
	if err := json.Unmarshal([]byte(jsonStr), &questions); err != nil {
		if len(images) > 0 {
			return &ParseResult{
				Success: false,
				Errors:  []string{"Model returned text that is not valid JSON. Coba lagi atau perkecil/ganti gambar."},
			}, nil
		}
		return ParseQuestionsSimple(text), nil
	}

	return &ParseResult{
		Success:   true,
		Questions: questions,
		Warnings:  []string{fmt.Sprintf("Parsed %d questions successfully", len(questions))},
	}, nil
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

// ensureFirstQuestionNumbered prepends "1. " when the text has no "N. " line so the
// line-based parser can open a question (users often paste the stem without "1.").
func ensureFirstQuestionNumbered(text string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		s := strings.TrimSpace(line)
		if s == "" || isMediaMarkerLine(s) {
			continue
		}
		if matched, _ := regexp.MatchString(`^\d+\.\s`, s); matched {
			return text
		}
		break
	}
	for i, line := range lines {
		s := strings.TrimSpace(line)
		if s == "" || isMediaMarkerLine(s) {
			continue
		}
		if matched, _ := regexp.MatchString(`^\d+\.\s`, s); matched {
			return text
		}
		lines[i] = "1. " + s
		return strings.Join(lines, "\n")
	}
	return text
}

// extractAfterAnswerKeywords returns text after "correct answer:" / "answer:" (case-insensitive).
func extractAfterAnswerKeywords(line string) string {
	s := strings.TrimSpace(line)
	low := strings.ToLower(s)
	for _, key := range []string{"correct answer:", "answer:"} {
		if idx := strings.Index(low, key); idx >= 0 {
			return strings.TrimSpace(s[idx+len(key):])
		}
	}
	return s
}

// ParseQuestionsSimple provides a simple rule-based parser as fallback
func ParseQuestionsSimple(text string) *ParseResult {
	text = ensureFirstQuestionNumbered(text)
	lines := strings.Split(text, "\n")
	var questions []ParsedQuestion
	var currentQuestion *ParsedQuestion
	var options []string
	var questionNumber int
	var warnings []string
	var pendingImageURLs []string

	flushPendingImages := func(q *ParsedQuestion) {
		if q == nil {
			return
		}
		for _, url := range pendingImageURLs {
			q.ImageURLs = appendUniqueURL(q.ImageURLs, url)
		}
		pendingImageURLs = nil
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if isMediaMarkerLine(line) {
			if m := regexp.MustCompile(`(?i)^\[!\s*(https?://[^\]]+)\]\s*$`).FindStringSubmatch(line); len(m) > 1 {
				url := strings.TrimRight(strings.TrimSpace(m[1]), "),.;")
				if url != "" {
					pendingImageURLs = append(pendingImageURLs, url)
				}
			} else if m := regexp.MustCompile(`(?i)https?://[^\s\]]+`).FindString(line); m != "" {
				pendingImageURLs = append(pendingImageURLs, strings.TrimRight(m, "),.;"))
			}
			continue
		}

		// Detect numbered question (e.g., "1.", "2.", etc.)
		if matched, _ := regexp.MatchString(`^\d+\.\s`, line); matched {
			// Save previous question
			if currentQuestion != nil {
				if currentQuestion.Type == "multiple-choice" && len(options) > 0 {
					currentQuestion.Options = options
				}
				questions = append(questions, *currentQuestion)
			}

			questionNumber++
			// Remove number prefix
			re := regexp.MustCompile(`^\d+\.\s`)
			content := re.ReplaceAllString(line, "")

			// Detect question type
			qType := "multiple-choice" // default
			if strings.Contains(strings.ToLower(content), "true or false") ||
				strings.Contains(strings.ToLower(content), "true/false") {
				qType = "true-false"
			}

			currentQuestion = &ParsedQuestion{
				Type:       qType,
				Title:      fmt.Sprintf("Question %d", questionNumber),
				Content:    content,
				Difficulty: "medium",
				Points:     5,
				Tags:       []string{"general"},
			}
			flushPendingImages(currentQuestion)
			options = []string{}
		} else if currentQuestion != nil {
			// Check for options (a), b), c), d) or a., b., c., d.)
			if matched, _ := regexp.MatchString(`^[a-d][\).\)]\s`, line); matched {
				optionRe := regexp.MustCompile(`^[a-d][\.\)]\s`)
				optionText := optionRe.ReplaceAllString(line, "")
				optionText = strings.TrimPrefix(optionText, ") ")
				optionText = strings.TrimSpace(optionText)
				if optionText != "" {
					options = append(options, optionText)
				}
			} else if strings.HasPrefix(strings.ToLower(line), "correct answer:") ||
				strings.HasPrefix(strings.ToLower(line), "answer:") {
				answerText := extractAfterAnswerKeywords(line)

				// Handle letter answers (a, b, c, d)
				if matched, _ := regexp.MatchString(`^[a-d]$`, strings.ToLower(answerText)); matched {
					if len(options) > 0 {
						ch := strings.ToLower(answerText)[0]
						index := int(ch - 'a')
						if index < len(options) {
							currentQuestion.CorrectAnswer = options[index]
						}
					} else {
						currentQuestion.CorrectAnswer = strings.ToUpper(answerText)
					}
				} else {
					currentQuestion.CorrectAnswer = answerText
				}

				// Set type based on answer
				if strings.ToLower(answerText) == "true" || strings.ToLower(answerText) == "false" {
					currentQuestion.Type = "true-false"
				}
			} else if strings.HasPrefix(strings.ToLower(line), "explanation:") {
				currentQuestion.Explanation = strings.TrimSpace(strings.TrimPrefix(line, "explanation:"))
			}
		}
	}

	// Save last question
	if currentQuestion != nil {
		if currentQuestion.Type == "multiple-choice" && len(options) > 0 {
			currentQuestion.Options = options
		}
		flushPendingImages(currentQuestion)
		questions = append(questions, *currentQuestion)
	}

	if len(questions) == 0 {
		return &ParseResult{
			Success:  false,
			Errors:   []string{"No questions could be parsed from the input text"},
			Warnings: warnings,
		}
	}

	if len(warnings) > 0 {
		warnings = append(warnings, "Used simple parser - AI parsing unavailable")
	}

	return &ParseResult{
		Success:   true,
		Questions: questions,
		Warnings:  warnings,
	}
}
