package auth

import (
	"os"
	"time"
	"yingce/backend/internal/kernel"
	"yingce/backend/internal/model"
)

const localGuestPasswordMarker = "!local-guest-no-password"

func IsLocalGuest(user *model.User) bool {
	return user != nil && user.Role == model.UserRoleUser && user.PasswordHash == localGuestPasswordMarker
}

// LocalGuestSession issues an ordinary, isolated, unprivileged session. No signup bonus.
func (s *Service) LocalGuestSession(context LoginContext) (*AuthSessionResult, error) {
	if os.Getenv("CANVAS_LOCAL_GUEST_ENABLED") != "true" {
		return nil, kernel.Forbidden("内网访客模式未开启")
	}
	count, err := s.repo.UserCount()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, kernel.Forbidden("请先初始化管理员，再开启访客模式")
	}
	now := time.Now()
	id := kernel.NewID()
	user := model.User{ID: id, Username: "guest_" + id, DisplayName: "内网访客", Role: model.UserRoleUser, Status: model.UserStatusActive, PasswordHash: localGuestPasswordMarker, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.Create(&user); err != nil {
		return nil, err
	}
	event := &model.LoginLog{ID: kernel.NewID(), Event: "login", Method: "local_guest", Source: "web", Result: "success", IP: context.IP, UserAgent: loginLogText(context.UserAgent, 512)}
	return s.createAuthSession(&user, event)
}
