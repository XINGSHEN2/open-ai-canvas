package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"yingce/backend/internal/model"
)

func TestLocalQwenUsesStandardImagesWireContract(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if strings.HasSuffix(r.URL.Path, "/generations") {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if _, ok := body["resolution"]; ok {
				t.Error("website resolution tier leaked to standard API")
			}
			if body["size"] != "1024x1024" || body["model"] != "qwen-image-2.1" {
				t.Errorf("bad generation body %v", body)
			}
		} else {
			if r.URL.Path != "/v1/images/edits" {
				t.Error(r.URL.Path)
			}
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
			}
			if r.FormValue("prompt") != "cube" || len(r.MultipartForm.File["image"]) != 1 {
				t.Error("missing multipart image")
			}
			if r.MultipartForm != nil {
				defer r.MultipartForm.RemoveAll()
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://example.com/result.png"}]}`))
	}))
	defer upstream.Close()
	profile := DefaultImageCapabilityConfig(string(model.ChannelInterfaceOpenAIImage), "qwen-image-2.1")
	profile.Quality.Supported = false
	input := canvasGenerationInput{Mode: "image", Prompt: "cube", Config: providerConfig{InterfaceType: string(model.ChannelInterfaceOpenAIImage), Model: "qwen-image-2.1", BaseURL: upstream.URL + "/v1", Size: "1024x1024", ImageResolution: "1k"}, ImageCapability: profile}
	ctx := ensureOfficialProtocolAdapter(context.Background(), input.Config.InterfaceType)
	if _, err := runImageTask(ctx, input); err != nil {
		t.Fatal(err)
	}
	input.ReferenceImages = []providerMedia{{Name: "reference.png", DataURL: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a3i8AAAAASUVORK5CYII="}}
	if _, err := runImageTask(ctx, input); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("unexpected upstream calls %d", calls)
	}
}
