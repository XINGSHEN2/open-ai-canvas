// Operator-only configuration using the existing encrypted channel store.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"yingce/backend/internal/app"
	"yingce/backend/internal/database"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func main() {
	apply := flag.Bool("apply", false, "install plugin and publish route")
	free := flag.Bool("provider-billed", false, "website credits are zero; provider account pays actual costs")
	flag.Parse()
	if !*apply || !*free || flag.NArg() != 1 {
		panic("requires --apply --provider-billed PROFILE_DIR after database backup")
	}
	dir := flag.Arg(0)
	key := strings.TrimSpace(os.Getenv("MOARK_API_KEY"))
	if key == "" {
		panic("missing existing provider key")
	}
	dataDir := os.Getenv("CANVAS_BACKEND_DATA_DIR")
	if dataDir == "" {
		panic("explicit data dir required")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "model.json"))
	must(err)
	var profile app.ChannelModelRequest
	must(json.Unmarshal(raw, &profile))
	if profile.ModelKey != "minimax-h3-gitee" || profile.Protocol != "gitee-h3-video" {
		panic("unexpected model")
	}
	_, err = app.NormalizeModelCapabilityConfigForModel("video", profile.Protocol, profile.ProviderModelKey, profile.CapabilityConfig)
	must(err)
	db, err := database.Open(database.Config{Driver: "sqlite", DataDir: dataDir})
	must(err)
	var admins []model.User
	must(db.Where("role = ? AND status = ?", model.UserRoleAdmin, model.UserStatusActive).Find(&admins).Error)
	if len(admins) != 1 {
		panic("requires exactly one active administrator")
	}
	svc := app.New(repository.New(db), dataDir)
	defer svc.Close()
	pkg, err := os.ReadFile(filepath.Join(dir, "gitee-h3.yingce-plugin"))
	must(err)
	_, err = svc.InstallPluginForAdmin(&admins[0], pkg, "gitee-h3.yingce-plugin")
	must(err)
	page, err := svc.AdminSystemChannelPage(&admins[0], app.AdminListQuery{Keyword: "Gitee H3", Limit: 100})
	must(err)
	id := ""
	for _, ch := range page.Channels {
		if ch.Name == "Gitee H3" {
			id = ch.ID
		}
	}
	enabled := true
	limit := 1
	if id == "" {
		channel, err := svc.CreateSystemChannel(&admins[0], app.ChannelRequest{Name: "Gitee H3", BaseURL: "https://ai.gitee.com", APIKey: key, Enabled: &enabled, ConcurrencyLimit: &limit})
		must(err)
		id = channel.ID
	}
	models, err := svc.AdminChannelModels(&admins[0], id)
	must(err)
	for _, mode := range []string{"reference_to_video", "text_to_video", "image_to_video"} {
		var item app.ChannelModelRequest
		must(json.Unmarshal(raw, &item))
		video := item.CapabilityConfig.Video
		video.Operations = []string{mode}
		video.DefaultOperation = mode
		switch mode {
		case "reference_to_video":
			item.DisplayName = "MiniMax-H3（Gitee · 参考生视频）"
		case "text_to_video":
			item.ModelKey = "minimax-h3-gitee-t2va"
			item.DisplayName = "MiniMax-H3（Gitee · 文生视频）"
			item.Description = "只发送文本提示词，不上传素材。固定 6 秒、16:9、20 步；上游账号承担调用费用。"
			video.References.MinImages = 0
			video.References.MaxImages = 0
			video.References.MaxVideos = 0
			video.References.MaxAudios = 0
		case "image_to_video":
			item.ModelKey = "minimax-h3-gitee-fl2va"
			item.DisplayName = "MiniMax-H3（Gitee · 首尾帧生视频）"
			item.Description = "需要两张图片：第 1 张首帧、第 2 张尾帧；画布显式首尾帧优先。图片经 Litterbox 公网中转 12 小时，提交前确认。"
			video.References.MinImages = 2
			video.References.MaxImages = 2
			video.References.MaxVideos = 0
			video.References.MaxAudios = 0
		}
		modelID := ""
		for _, m := range models {
			if m.ModelKey == item.ModelKey {
				modelID = m.ID
			}
		}
		saved, err := svc.SaveAdminChannelModel(&admins[0], id, modelID, item)
		must(err)
		fmt.Printf("Configured channel=%s model=%s; provider pays real usage, website credits=0\n", id, saved.ModelKey)
	}
}
