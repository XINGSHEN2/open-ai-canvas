package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiniMaxResumePollsExistingTaskWithoutCreate(t *testing.T) {
	allowLoopbackProviderTest(t)
	center, err := newPluginRuntime(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := withProtocolRegistry(context.Background(), center.registrySnapshot())
	adapter, ok := declarativeProtocolAdapterForContext(ctx, "minimax-video")
	if !ok {
		t.Fatal("MiniMax adapter unavailable")
	}
	creates, polls, downloads := 0, 0, 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/video_generation":
			creates++
			http.Error(w, "must not submit again", 500)
		case "/v2/query/video_generation/existing-h3":
			polls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"task":{"id":"existing-h3","status":"succeeded","content":{"url":"` + server.URL + `/result.mp4"}}}`))
		case "/result.mp4":
			downloads++
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("video"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ctx = context.WithValue(ctx, providerAnalyticsKey{}, providerAnalyticsContext{ProviderRequestID: "existing-h3"})
	input := canvasGenerationInput{Mode: "video", Prompt: "test", Config: providerConfig{BaseURL: server.URL, APIKey: "test", Model: "MiniMax-H3", InterfaceType: "minimax-video"}}
	result, err := runProtocolAdapterTaskWithPolicy(ctx, input, adapter, fastVideoPollPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if creates != 0 || polls != 1 || downloads != 1 || result["mode"] != "video" {
		t.Fatalf("create=%d poll=%d download=%d result=%v", creates, polls, downloads, result)
	}
}
