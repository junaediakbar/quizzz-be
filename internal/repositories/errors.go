package repositories

import "errors"

var (
	ErrExamNotFound    = errors.New("exam not found")
	ErrQuestionNotFound = errors.New("question not found")
	ErrSessionNotFound  = errors.New("session not found")
	ErrResultNotFound   = errors.New("result not found")
	ErrUserNotFound     = errors.New("user not found")
	ErrBankNotFound     = errors.New("question bank not found")
)
