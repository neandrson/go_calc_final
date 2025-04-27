package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/neandrson/go_calc_final/internal/lib/jwt"
	"github.com/neandrson/go_calc_final/internal/lib/logger/sl"
	"github.com/neandrson/go_calc_final/internal/models"
	"github.com/neandrson/go_calc_final/internal/repositories"
	"golang.org/x/crypto/bcrypt"
)

type IOAuth interface {
	Login(
		ctx context.Context,
		login string,
		password string,
		appID int,
	) (token string, err error)
	RegisterNewUser(
		ctx context.Context,
		login string,
		password string,
	) (userID int64, err error)
}

type Auth struct {
	log         *slog.Logger
	usrSaver    UserSaver
	usrProvider UserProvider
	appProvider AppProvider
	tokenTTL    time.Duration
}

var (
	ErrInvalidCredentials = errors.New("недействительные учетные данные")
)

//go:generate go run github.com/vektra/mockery/v2@v2.28.2 --name=URLSaver
type UserSaver interface {
	Create(ctx context.Context, login string, passHash []byte) (int64, error)
}

type UserProvider interface {
	Get(ctx context.Context, login string) (models.User, error)
}

type AppProvider interface {
	App(ctx context.Context, appID int) (models.App, error)
}

func New(
	log *slog.Logger,
	userSaver UserSaver,
	userProvider UserProvider,
	appProvider AppProvider,
	tokenTTL time.Duration,
) *Auth {
	return &Auth{
		usrSaver:    userSaver,
		usrProvider: userProvider,
		log:         log,
		appProvider: appProvider,
		tokenTTL:    tokenTTL,
	}
}

// Login checks if user with given credentials exists in the system and returns access token.
//
// If user exists, but password is incorrect, returns error.
// If user doesn't exist, returns error.
func (a *Auth) Login(
	ctx context.Context,
	login string,
	password string,
	appID int,
) (string, error) {
	const op = "Auth.Login"

	log := a.log.With(
		slog.String("op", op),
		slog.String("username", login),
	)

	log.Info("попытка входа пользователя")

	user, err := a.usrProvider.Get(ctx, login)
	if err != nil {
		if errors.Is(err, repositories.ErrUserNotFound) {
			a.log.Warn("пользователь не найден", sl.Err(err))

			return "", fmt.Errorf("%s: %w", op, ErrInvalidCredentials)
		}

		a.log.Error("пользователь не доступен", sl.Err(err))

		return "", fmt.Errorf("%s: %w", op, err)
	}

	if err := bcrypt.CompareHashAndPassword(user.PassHash, []byte(password)); err != nil {
		a.log.Info("недействительные учетные данные", sl.Err(err))

		return "", fmt.Errorf("%s: %w", op, ErrInvalidCredentials)
	}

	app, err := a.appProvider.App(ctx, appID)
	if err != nil {
		a.log.Info("приложение не доступно", sl.Err(err))

		return "", fmt.Errorf("%s: %w", op, err)
	}

	log.Info("пользователь успешно вошел в систему")

	token, err := jwt.NewToken(user, app, a.tokenTTL)
	if err != nil {
		a.log.Error("не удалось сгенерировать токен", sl.Err(err))

		return "", fmt.Errorf("%s: %w", op, err)
	}

	return token, nil
}

// RegisterNewUser registers new user in the system and returns user ID.
// If user with given username already exists, returns error.
func (a *Auth) RegisterNewUser(ctx context.Context, login string, pass string) (int64, error) {
	const op = "Auth.RegisterNewUser"

	log := a.log.With(
		slog.String("op", op),
		slog.String("login", login),
	)

	log.Info("пользователь зарегистриарован")

	passHash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		log.Error("не удалось сгенерировать хэш пароля", sl.Err(err))

		return 0, fmt.Errorf("%s: %w", op, err)
	}

	id, err := a.usrSaver.Create(ctx, login, passHash)
	if err != nil {
		log.Error("не удалось сохранить пользователя", sl.Err(err))

		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return id, nil
}
