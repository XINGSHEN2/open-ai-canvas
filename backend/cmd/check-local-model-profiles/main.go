package main

import (
 "context"
 "encoding/json"
 "fmt"
 "os"
 "yingce/backend/internal/app"
 "yingce/backend/internal/protocol"
)

func main() {
 data, err := os.ReadFile(os.Args[1]); if err != nil { panic(err) }
 var input struct { Profiles []struct { Model app.ChannelModelRequest `json:"model"` } `json:"profiles"` }
 if err := json.Unmarshal(data, &input); err != nil { panic(err) }
 raw, err := os.ReadFile("../plugin-packages/minimax-hailuo-video-v2/manifest.json"); if err != nil { panic(err) }
 adapters, err := protocol.LoadInstalledProviders(raw, nil); if err != nil { panic(err) }
 registry, err := protocol.NewRegistry(adapters...); if err != nil { panic(err) }
 imageRaw, err := os.ReadFile("../plugin-packages/openai-images/manifest.json"); if err != nil { panic(err) }
 imageAdapters, err := protocol.LoadInstalledProviders(imageRaw,nil); if err != nil { panic(err) }
 for _, adapter := range imageAdapters { if err := registry.Register(adapter); err != nil { panic(err) } }
 for _, item := range input.Profiles {
   model := item.Model
   config, err := app.NormalizeModelCapabilityConfigForModel(model.Capability, model.Protocol, model.ProviderModelKey, model.CapabilityConfig,registry)
   if err != nil { panic(fmt.Errorf("%s: %w", model.ModelKey, err)) }
   if config.Image != nil {
     if config.Image.References.MaskSupported || config.Image.Quality.Supported || config.Image.TransparentBackground.Supported || config.Image.MaxOutputs!=1 || config.Image.References.MaxImages!=10 { panic("unsafe Qwen image controls") }
   }
   if config.Video != nil {
     adapter, ok := registry.Resolve(model.Protocol); if !ok { panic("video adapter missing") }
     req := protocol.GenerationRequest{Capability:"video", Model:model.ProviderModelKey, Prompt:"test", Duration:5, Resolution:"480P", AspectRatio:"16:9"}
     req.Images = []protocol.MediaReference{{Kind:"image",Role:"reference_image",URL:"https://example.com/reference.png",Order:0}}
     spec, err := adapter.BuildCreate(context.Background(), protocol.RequestContext{BaseURL:"http://10.42.1.2:30011",Request:req})
     if err != nil { panic(err) }
     encoded, err := json.Marshal(spec.Body); if err != nil { panic(err) }
     var body map[string]any
     if err := json.Unmarshal(encoded,&body); err != nil { panic(err) }
     if spec.Path!="/v2/video_generation" || body["resolution"]!="480P" || body["ratio"]!="16:9" { panic("incorrect H3 wire request") }
     fmt.Println("H3 wire:",string(encoded))
     if len(config.Video.Resolutions)!=2 || config.Video.Duration.Max!=15 { panic("unsafe H3 profile") }
   }
   fmt.Println("valid:", model.ModelKey)
 }
}
