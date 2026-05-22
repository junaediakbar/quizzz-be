package present

import (
	"encoding/json"
	"strings"

	"github.com/junaediakbar/quizzz-backend/models"
)

// QuestionForStudentJSON builds a safe question payload (no correct answer).
func QuestionForStudentJSON(q *models.Question, examPoints int) map[string]interface{} {
	opts := OptionsSlice(q.Options)
	tags := tagsSlice(q.Tags)
	pts := examPoints
	if pts <= 0 {
		pts = q.Points
	}
	m := map[string]interface{}{
		"id":          q.ID,
		"type":        q.Type,
		"title":       q.Title,
		"content":     q.Content,
		"difficulty":  q.Difficulty,
		"points":      pts,
		"created_by":  q.CreatedBy,
		"created_at":  q.CreatedAt,
		"updated_at":  q.UpdatedAt,
	}
	if len(opts) > 0 {
		m["options"] = opts
	}
	if len(tags) > 0 {
		m["tags"] = tags
	}
	imgs := ParseQuestionImages(q.ImageURLs)
	if len(imgs) > 0 {
		m["images"] = imgs
		if flat := FlatImageURLs(imgs); len(flat) > 0 {
			m["image_urls"] = flat
		}
	}
	if q.CategoryID != nil {
		m["category_id"] = *q.CategoryID
	}
	return m
}

// OptionsSlice parses the JSON options field into a slice.
func OptionsSlice(opt *string) []string {
	if opt == nil || *opt == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(*opt), &out); err != nil {
		return nil
	}
	return out
}

func tagsSlice(tags *string) []string {
	if tags == nil || *tags == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(*tags), &out); err != nil {
		return nil
	}
	return out
}

// ImageURLsSlice returns flat URLs for backward-compatible clients.
func ImageURLsSlice(raw *string) []string {
	return FlatImageURLs(ParseQuestionImages(raw))
}

// QuestionFullJSON returns a teacher/admin view with answers.
func QuestionFullJSON(q *models.Question) map[string]interface{} {
	m := QuestionForStudentJSON(q, q.Points)
	m["correct_answer"] = q.CorrectAnswer
	if q.Explanation != nil {
		m["explanation"] = *q.Explanation
	}
	return m
}

// NormalizeAnswer compares student response to key for auto-grading.
func AnswersMatch(qType, student, correct string) bool {
	s := strings.TrimSpace(student)
	c := strings.TrimSpace(correct)
	switch qType {
	case "true-false":
		return strings.EqualFold(s, c)
	case "multiple-choice":
		return strings.EqualFold(s, c)
	case "short-answer", "fill-blank":
		return strings.EqualFold(strings.TrimSpace(s), strings.TrimSpace(c))
	default:
		return false
	}
}
