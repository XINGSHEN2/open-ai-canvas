package auth

import (
	"testing"
	"yingce/backend/internal/model"
)

func TestPasswordOnlyRegistration(t *testing.T) {
	for _, tc := range []struct {
		name, config string
		ok           bool
	}{
		{"enabled", `{"enabled":true,"passwordOnly":true}`, true},
		{"closed", `{"enabled":false,"passwordOnly":true}`, false},
		{"default-off", `{"enabled":true}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, db := newRegistrationTestService(t)
			db.Create(&model.User{ID: "admin", Username: "admin", Role: model.UserRoleAdmin, Status: model.UserStatusActive})
			db.Create(&model.SystemSetting{Key: registrationSettingKey, ValueJSON: tc.config})
			req := RegisterRequest{Username: "member", Password: "password123", AcceptedTerms: true}
			result, err := svc.Register(req)
			if !tc.ok {
				if err == nil {
					t.Fatal("registration unexpectedly allowed")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.User.Role != model.UserRoleUser || result.User.Email != "" || result.User.Phone != "" || result.User.EmailVerifiedAt != nil || result.User.PhoneVerifiedAt != nil {
				t.Fatal("invalid ordinary user")
			}
			if _, err = svc.Register(req); err == nil {
				t.Fatal("duplicate username accepted")
			}
			req.Username = "another"
			req.Password = "x"
			if _, err = svc.Register(req); err == nil {
				t.Fatal("weak password accepted")
			}
			req.Password = "password123"
			req.AcceptedTerms = false
			if _, err = svc.Register(req); err == nil {
				t.Fatal("missing agreement accepted")
			}
			req.AcceptedTerms = true
			req.Email = "unverified@example.com"
			if _, err = svc.Register(req); err == nil {
				t.Fatal("unverified contact accepted")
			}
		})
	}
}

func TestPasswordOnlySettingPreserved(t *testing.T) {
	svc, db := newRegistrationTestService(t)
	admin := &model.User{ID: "admin", Username: "admin", Role: model.UserRoleAdmin, Status: model.UserStatusActive}
	db.Create(admin)
	yes := true
	if _, err := svc.UpdateRegistrationSetting(admin, RegistrationSettingRequest{Enabled: true, PasswordOnly: &yes}); err != nil {
		t.Fatal(err)
	}
	v, err := svc.UpdateRegistrationSetting(admin, RegistrationSettingRequest{Enabled: false})
	if err != nil || !v.PasswordOnly || v.Enabled {
		t.Fatalf("lost setting: %v %v", v, err)
	}
}
