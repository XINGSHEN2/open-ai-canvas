// Explicit operator-side provisioning. Does not expose a new unauthenticated HTTP API.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"yingce/backend/internal/app"
	"yingce/backend/internal/database"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
)

func main() {
	apply := flag.Bool("apply", false, "write validated model configuration")
	free := flag.Bool("free-internal", false, "explicitly publish these internal routes at zero credits")
	flag.Parse()
	if !*apply || !*free || flag.NArg() != 1 {
		panic("requires --apply --free-internal CONFIG; back up the database first")
	}
	raw, err := os.ReadFile(flag.Arg(0))
	must(err)
	var config struct {
		Profiles []struct {
			Channel app.ChannelRequest      `json:"channel"`
			Model   app.ChannelModelRequest `json:"model"`
		} `json:"profiles"`
	}
	must(json.Unmarshal(raw, &config))
	if len(config.Profiles) != 2 {
		panic("expected exactly two local profiles")
	}
	dataDir := os.Getenv("CANVAS_BACKEND_DATA_DIR")
	if dataDir == "" {
		panic("explicit data dir required")
	}
	db, err := database.Open(database.Config{Driver: "sqlite", DataDir: dataDir})
	must(err)
	var admins []model.User
	must(db.Where("role = ? AND status = ?", model.UserRoleAdmin, model.UserStatusActive).Find(&admins).Error)
	if len(admins) != 1 {
		panic("requires exactly one active administrator; select the operator explicitly otherwise")
	}
	svc := app.New(repository.New(db), dataDir)
	defer svc.Close()
	actor := &admins[0]
	for _, profile := range config.Profiles {
		if profile.Channel.BaseURL != "http://10.42.1.1:9008/v1" && profile.Channel.BaseURL != "http://10.42.1.2:30011" {
			panic("unexpected upstream")
		}
		_, err := app.NormalizeModelCapabilityConfigForModel(profile.Model.Capability, profile.Model.Protocol, profile.Model.ProviderModelKey, profile.Model.CapabilityConfig)
		must(err)
	}
	for _, profile := range config.Profiles {
		page, err := svc.AdminSystemChannelPage(actor, app.AdminListQuery{Keyword: profile.Channel.Name, Limit: 100})
		must(err)
		id := ""
		for _, channel := range page.Channels {
			if channel.Name == profile.Channel.Name {
				id = channel.ID
				break
			}
		}
		if id == "" {
			channel, err := svc.CreateSystemChannel(actor, profile.Channel)
			must(err)
			id = channel.ID
		}
		existing, err := svc.AdminChannelModels(actor, id)
		must(err)
		modelID := ""
		for _, item := range existing {
			if item.ModelKey == profile.Model.ModelKey {
				modelID = item.ID
				break
			}
		}
		profile.Model.PriceConfigured = true
		profile.Model.UnitPriceMicrocredits = 0
		item, err := svc.SaveAdminChannelModel(actor, id, modelID, profile.Model)
		must(err)
		fmt.Printf("provisioned channel=%s model=%s key=%s free_internal=true\n", id, item.ID, item.ModelKey)
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
