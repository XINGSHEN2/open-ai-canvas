package auth

import (
	"testing"
	"yingce/backend/internal/model"
)

func TestLocalGuestSessionsAreIsolatedAndUnprivileged(t *testing.T) {
	t.Setenv("CANVAS_LOCAL_GUEST_ENABLED", "true")
	svc, db := newRegistrationTestService(t)
	if err := db.Create(&model.User{ID: "admin", Username: "admin", Role: model.UserRoleAdmin, Status: model.UserStatusActive}).Error; err != nil {
		t.Fatal(err)
	}
	one, err := svc.LocalGuestSession(LoginContext{IP: "10.42.1.2"})
	if err != nil {
		t.Fatal(err)
	}
	two, err := svc.LocalGuestSession(LoginContext{IP: "10.42.1.2"})
	if err != nil {
		t.Fatal(err)
	}
	if one.User.ID == two.User.ID || one.Session == two.Session {
		t.Fatal("guest identities are shared")
	}
	user, err := svc.CurrentUser(one.Session)
	if err != nil {
		t.Fatal(err)
	}
	if user.Role != model.UserRoleUser || !IsLocalGuest(user) || verifyPassword("", user.PasswordHash) {
		t.Fatal("unsafe guest privileges/password")
	}
	var admins int64
	if err := db.Model(&model.User{}).Where("role = ?", model.UserRoleAdmin).Count(&admins).Error; err != nil || admins != 1 {
		t.Fatal("guest promoted to administrator")
	}
}

func TestLocalGuestDisabledDoesNotCreateUsers(t *testing.T) {
	t.Setenv("CANVAS_LOCAL_GUEST_ENABLED", "false")
	svc, db := newRegistrationTestService(t)
	if _, err := svc.LocalGuestSession(LoginContext{}); err == nil {
		t.Fatal("guest mode enabled by default")
	}
	var count int64
	db.Model(&model.User{}).Count(&count)
	if count != 0 {
		t.Fatal("disabled guest wrote user")
	}
}
