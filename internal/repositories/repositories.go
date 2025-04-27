package repositories

import "errors"

var (
	ErrUserExists         = errors.New("пользователь уже существует")
	ErrUserNotFound       = errors.New("пользователь не найден")
	ErrAppNotFound        = errors.New("приложение не найдено")
	ErrExpressionNotFound = errors.New("выражение не найдено")
)
