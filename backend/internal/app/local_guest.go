package app

import (
	"yingce/backend/internal/auth"
	"yingce/backend/internal/model"
)

func (s *Service) LocalGuestSession(context LoginContext) (*AuthSessionResult, error) {
	return s.authDomain().LocalGuestSession(context)
}

func (s *Service) IsLocalGuest(user *model.User) bool { return auth.IsLocalGuest(user) }

// Only resolved, administrator-provisioned free local routes may be used by guests.
func (s *Service) validateLocalGuestTask(userID string, input map[string]any) error {
	user, err := s.repo.User(userID)
	if err != nil {
		return err
	}
	if !auth.IsLocalGuest(user) {
		return nil
	}
	config, ok := input["config"].(map[string]any)
	if !ok {
		return Forbidden("访客只能使用本地共享模型")
	}
	key, _ := config["channelModelKey"].(string)
	channelID, _ := config["channelId"].(string)
	if channelID == "" {
		return Forbidden("访客只能使用本地共享模型")
	}
	channel, err := s.repo.SystemChannel(channelID)
	if err != nil {
		return err
	}
	base := channel.BaseURL
	protocol, _ := config["interfaceType"].(string)
	mode, _ := input["mode"].(string)
	if !((key == "qwen-image-2.1" && protocol == "openai-image" && mode == "image") || (key == "minimax-h3-ref2va" && protocol == "minimax-video" && mode == "video")) {
		return Forbidden("访客不能选择其它协议或执行器")
	}
	if channelID == "" || !((base == "http://10.42.1.1:9008/v1" && key == "qwen-image-2.1") || (base == "http://10.42.1.2:30011" && key == "minimax-h3-ref2va")) {
		return Forbidden("访客只能使用本地共享模型")
	}
	return nil
}
