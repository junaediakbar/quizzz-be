package present

import (
	"encoding/json"
	"fmt"
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
		"id":         q.ID,
		"type":       q.Type,
		"title":      q.Title,
		"content":    q.Content,
		"difficulty": q.Difficulty,
		"points":     pts,
		"created_by": q.CreatedBy,
		"created_at": q.CreatedAt,
		"updated_at": q.UpdatedAt,
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

// StepAnswersSlice parses the JSON array of per-step correct answers
// (stored in the correct_answer column for "step-by-step" questions).
func StepAnswersSlice(correct string) []string {
	s := strings.TrimSpace(correct)
	if s == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

// StepAnswersFromStudent parses a student's step-by-step response
// (a JSON object mapping step number -> answer, e.g. {"1":"20","2":"45"}).
// Falls back to treating the raw string as the answer to step 1.
func StepAnswersFromStudent(student string) map[string]string {
	s := strings.TrimSpace(student)
	if strings.HasPrefix(s, "{") {
		var m map[string]string
		if err := json.Unmarshal([]byte(s), &m); err == nil {
			return m
		}
	}
	return nil
}

// StepAnswerMatches compares one student step to its key (case/whitespace tolerant).
func StepAnswerMatches(student, correct string) bool {
	norm := func(v string) string {
		v = strings.TrimSpace(v)
		v = strings.Trim(v, "\"'")
		v = strings.Join(strings.Fields(v), " ")
		return strings.ToLower(v)
	}
	return norm(student) == norm(correct)
}

// StepByStepGrade grades a step-by-step response.
// Returns earned points, whether ALL steps are correct, and per-step detail
// (index, student answer, key, correct?) for feedback.
func StepByStepGrade(student, correct string, keys []string) (correctSteps int, details []map[string]interface{}) {
	byKey := StepAnswersFromStudent(student)
	details = make([]map[string]interface{}, 0, len(keys))
	for i, key := range keys {
		stepNum := fmt.Sprintf("%d", i+1)
		stu := ""
		if byKey != nil {
			stu = byKey[stepNum]
		} else if i == 0 {
			// legacy/plain answer: treat as step 1
			stu = student
		}
		ok := strings.TrimSpace(stu) != "" && StepAnswerMatches(stu, key)
		if ok {
			correctSteps++
		}
		details = append(details, map[string]interface{}{
			"step":     i + 1,
			"student":  stu,
			"key":      key,
			"correct":  ok,
			"answered": strings.TrimSpace(stu) != "",
		})
	}
	return correctSteps, details
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
