package authgrpc

import (
	"context"
	"errors"

	"github.com/neandrson/go_calc_final/internal/repositories"
	"github.com/neandrson/go_calc_final/internal/services/auth"
	authv1 "github.com/s0vunia/protos/gen/go/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type serverAPI struct {
	authv1.UnimplementedAuthServer
	auth auth.IOAuth
}

func Register(gRPCServer *grpc.Server, auth auth.IOAuth) {
	authv1.RegisterAuthServer(gRPCServer, &serverAPI{auth: auth})
}

func (s *serverAPI) Login(
	ctx context.Context,
	in *authv1.LoginRequest,
) (*authv1.LoginResponse, error) {
	if in.Login == "" {
		return nil, status.Error(codes.InvalidArgument, "требуется вход")
	}

	if in.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "требуется пароль")
	}

	if in.GetAppId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "app_id обязателен")
	}

	token, err := s.auth.Login(ctx, in.GetLogin(), in.GetPassword(), int(in.GetAppId()))
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			return nil, status.Error(codes.InvalidArgument, "неверный логин или пароль")
		}

		return nil, status.Error(codes.Internal, "не удалось войти")
	}
	return &authv1.LoginResponse{Token: token}, nil
}

func (s *serverAPI) Register(
	ctx context.Context,
	in *authv1.RegisterRequest,
) (*authv1.RegisterResponse, error) {
	if in.Login == "" {
		return nil, status.Error(codes.InvalidArgument, "требуется вход")
	}

	if in.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "требуется пароль")
	}

	uid, err := s.auth.RegisterNewUser(ctx, in.GetLogin(), in.GetPassword())
	if err != nil {
		if errors.Is(err, repositories.ErrUserExists) {
			return nil, status.Error(codes.AlreadyExists, "пользователь уже существует")
		}

		return nil, status.Error(codes.Internal, "не удалось зарегистрировать пользователя")
	}

	return &authv1.RegisterResponse{UserId: uid}, nil
}
