package present

import (
	"encoding/json"

	"github.com/junaediakbar/quizzz-backend/internal/repositories"
	"github.com/junaediakbar/quizzz-backend/models"
)

// StudentCanViewResults enforces exam.config.show_results for students.
func StudentCanViewResults(cfg models.ExamConfig, res *models.ExamResult) bool {
	switch cfg.ShowResults {
	case "manual", "after-review":
		return res.GradedAt != nil
	default: // immediate
		return true
	}
}

// BuildResultDetail builds JSON for GET /results/:id and GET /sessions/:id/result.
func BuildResultDetail(
	exam *models.Exam,
	res *models.ExamResult,
	reviews []models.AnswerReview,
	questions map[string]*models.Question,
	role string,
) map[string]interface{} {
	pct := 0.0
	if res.MaxScore > 0 {
		pct = float64(res.Score) * 100 / float64(res.MaxScore)
	}

	canView := role == "teacher" || role == "admin" || StudentCanViewResults(exam.Config, res)
	showKeys := canView && (role == "teacher" || role == "admin" || exam.Config.AllowReview)

	examTitle := res.ExamTitle
	if examTitle == "" {
		examTitle = exam.Title
	}
	examGrade := res.ExamGrade
	if examGrade == "" {
		examGrade = exam.Grade
	}

	out := map[string]interface{}{
		"id":               res.ID,
		"session_id":       res.SessionID,
		"exam_id":          res.ExamID,
		"exam_title":       examTitle,
		"exam_grade":       examGrade,
		"student_id":       res.StudentID,
		"student_name":     res.StudentName,
		"score":            res.Score,
		"max_score":        res.MaxScore,
		"percentage":       pct,
		"passed":           res.Passed,
		"time_spent":       res.TimeSpent,
		"submitted_at":     res.SubmittedAt,
		"graded_at":        res.GradedAt,
		"results_visible":  canView,
		"allow_review":     exam.Config.AllowReview,
		"show_results":     exam.Config.ShowResults,
		"passing_score":    exam.Config.PassingScore,
		"answers":          []interface{}{},
	}

	if !canView {
		out["message"] = "Hasil ujian belum dipublikasikan. Silakan tunggu penilaian dari guru."
		return out
	}

	answerRows := make([]interface{}, 0, len(reviews))
	for _, rev := range reviews {
		row := map[string]interface{}{
			"session_id":     rev.SessionID,
			"question_id":    rev.QuestionID,
			"student_answer": rev.StudentAnswer,
			"is_correct":     rev.IsCorrect,
			"points":         rev.Points,
			"max_points":     rev.MaxPoints,
			"feedback":       rev.Feedback,
		}

		q := questions[rev.QuestionID]
		pendingReview := false
		if q != nil && (q.Type == "essay" || q.Type == "matching") && res.GradedAt == nil && rev.Points == 0 {
			pendingReview = true
		}
		row["pending_review"] = pendingReview

		if showKeys {
			row["correct_answer"] = rev.CorrectAnswer
		}

		if q != nil {
			qm := map[string]interface{}{
				"id":      q.ID,
				"type":    q.Type,
				"title":   q.Title,
				"content": q.Content,
				"points":  rev.MaxPoints,
			}
			if q.Options != nil && *q.Options != "" {
				var opts []string
				if err := json.Unmarshal([]byte(*q.Options), &opts); err == nil {
					qm["options"] = opts
				}
			}
			if q.ImageURLs != nil && *q.ImageURLs != "" {
				var imgs []string
				if err := json.Unmarshal([]byte(*q.ImageURLs), &imgs); err == nil {
					qm["image_urls"] = imgs
				}
			}
			if showKeys && q.Explanation != nil {
				qm["explanation"] = *q.Explanation
			}
			row["question"] = qm
		}

		answerRows = append(answerRows, row)
	}
	out["answers"] = answerRows
	return out
}

// EnrichResultListItem adds exam_title, exam_grade, and percentage to a list row.
func EnrichResultListItem(res *models.ExamResult) map[string]interface{} {
	pct := 0.0
	if res.MaxScore > 0 {
		pct = float64(res.Score) * 100 / float64(res.MaxScore)
	}
	return map[string]interface{}{
		"id":           res.ID,
		"session_id":   res.SessionID,
		"exam_id":      res.ExamID,
		"exam_title":   res.ExamTitle,
		"exam_grade":   res.ExamGrade,
		"student_id":   res.StudentID,
		"student_name": res.StudentName,
		"score":        res.Score,
		"max_score":    res.MaxScore,
		"percentage":   pct,
		"passed":       res.Passed,
		"time_spent":   res.TimeSpent,
		"submitted_at": res.SubmittedAt,
		"graded_at":    res.GradedAt,
	}
}

// LoadQuestionsForReviews fetches questions referenced by answer reviews.
func LoadQuestionsForReviews(
	questionRepo *repositories.QuestionRepository,
	reviews []models.AnswerReview,
) map[string]*models.Question {
	out := make(map[string]*models.Question)
	for _, rev := range reviews {
		if _, ok := out[rev.QuestionID]; ok {
			continue
		}
		q, err := questionRepo.FindByID(rev.QuestionID)
		if err == nil {
			out[rev.QuestionID] = q
		}
	}
	return out
}
