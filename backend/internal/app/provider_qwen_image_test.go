package app

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"yingce/backend/internal/model"
)

func qwenTestInput(baseURL string) canvasGenerationInput {
	profile := DefaultImageCapabilityConfig(string(model.ChannelInterfaceOpenAIImage), "qwen-image-2.1")
	profile.Quality.Supported = false
	return canvasGenerationInput{Mode: "image", Prompt: "synthetic cube", Config: providerConfig{InterfaceType: string(model.ChannelInterfaceOpenAIImage), Model: "qwen-image-2.1", BaseURL: baseURL + "/v1", APIKey: "test", Size: "1024x1024", ImageResolution: "1k"}, ImageCapability: profile}
}

func TestQwenAsyncImageRoundTripWire(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	png := mediaTestPNG(t)
	for _, refs := range []int{0, 1, 2} {
		t.Run(fmt.Sprintf("references-%d", refs), func(t *testing.T) {
			posts, polls, downloads := 0, 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodPost:
					posts++
					if r.Header.Get("Prefer") != "respond-async" {
						t.Error("missing async preference")
					}
					if refs == 0 {
						if r.URL.Path != "/v1/images/generations" {
							t.Error(r.URL.Path)
						}
						var body map[string]any
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Error(err)
						}
						if body["n"] != float64(1) || body["size"] != "1024x1024" || body["model"] != "qwen-image-2.1" || body["response_format"] != "b64_json" {
							t.Errorf("bad JSON fields: %v", body)
						}
						if _, ok := body["resolution"]; ok {
							t.Error("resolution leaked")
						}
					} else {
						if r.URL.Path != "/v1/images/edits" {
							t.Error(r.URL.Path)
						}
						if err := r.ParseMultipartForm(1 << 20); err != nil {
							t.Error(err)
							return
						}
						defer r.MultipartForm.RemoveAll()
						if r.FormValue("n") != "1" || r.FormValue("size") != "1024x1024" || r.FormValue("model") != "qwen-image-2.1" || r.FormValue("response_format") != "b64_json" || r.FormValue("prompt") != "synthetic cube" {
							t.Error("bad multipart fields")
						}
						if len(r.MultipartForm.File["image"]) != refs || len(r.MultipartForm.File) != 1 {
							t.Errorf("wrong file field/count: %v", r.MultipartForm.File)
						}
						for i, header := range r.MultipartForm.File["image"] {
							file, err := header.Open()
							if err != nil {
								t.Error(err)
								continue
							}
							data, err := io.ReadAll(file)
							file.Close()
							if err != nil || !bytes.Equal(data, png) || header.Filename != fmt.Sprintf("reference-ref-%d.png", i) {
								t.Error("reference bytes/order changed")
							}
						}
					}
					w.WriteHeader(http.StatusAccepted)
					_, _ = w.Write([]byte(`{"id":"qwen-one","task_id":"qwen-one","status":"queued","progress":0}`))
				case r.URL.Path == "/v1/images/qwen-one":
					polls++
					_, _ = w.Write([]byte(`{"id":"qwen-one","status":"done"}`))
				case r.URL.Path == "/v1/images/qwen-one/content":
					downloads++
					w.Header().Set("Content-Type", "image/png")
					_, _ = w.Write(png)
				default:
					t.Error(r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			input := qwenTestInput(server.URL)
			for i := 0; i < refs; i++ {
				input.ReferenceImages = append(input.ReferenceImages, providerMedia{ID: fmt.Sprintf("ref-%d", i), Name: fmt.Sprintf("ref-%d.png", i), DataURL: dataURL("image/png", png)})
			}
			result, err := runImageTask(context.Background(), input)
			if err != nil || result["mode"] != "image" || posts != 1 || polls != 1 || downloads != 1 {
				t.Fatalf("result=%v err=%v post=%d poll=%d download=%d", result, err, posts, polls, downloads)
			}
		})
	}
}

func TestQwenAsyncDeadlinePersistsAndFreshServiceResumesSameID(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	fixture, db := newMediaRecoveryTestService(t)
	s := New(fixture.repo, fixture.dataDir)
	png := mediaTestPNG(t)
	var posts, polls, downloads atomic.Int32
	var completed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost:
			posts.Add(1)
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"durable-qwen","status":"queued"}`))
		case r.URL.Path == "/v1/images/durable-qwen":
			polls.Add(1)
			if completed.Load() {
				_, _ = w.Write([]byte(`{"id":"durable-qwen","status":"done"}`))
			} else {
				_, _ = w.Write([]byte(`{"id":"durable-qwen","status":"running"}`))
			}
		case r.URL.Path == "/v1/images/durable-qwen/content":
			downloads.Add(1)
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	input := qwenTestInput(server.URL)
	task := seedMediaTask(t, db, input.Config)
	started := time.Now()
	if err := db.Model(task).Update("started_at", started).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(withProviderAnalytics(context.Background(), s, *task), 300*time.Millisecond)
	defer cancel()
	result, err := runImageTask(ctx, input)
	if !errors.Is(err, context.DeadlineExceeded) || result != nil {
		t.Fatalf("timeout became success: %v %v", result, err)
	}
	saved, err := s.repo.Task(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ProviderRequestID != "durable-qwen" || saved.Status != model.TaskStatusRunning || !s.shouldDeferProviderTask(*saved, task.InputJSON, context.DeadlineExceeded) {
		t.Fatalf("not recoverable: %+v", saved)
	}
	if err := s.repo.DeferRunningTaskForProviderPoll(saved.ID, saved.LeaseOwner, "后台仍在生成图片", 0); err != nil {
		t.Fatal(err)
	}
	restarted := New(s.repo, s.dataDir)
	claimed, err := restarted.repo.ClaimNextTask("worker-after-restart", time.Minute)
	if err != nil || claimed == nil || claimed.ID != task.ID || claimed.ProviderRequestID != "durable-qwen" {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	completed.Store(true)
	// References need not remain readable after restart: recovery does not build
	// multipart or hydrate them again, it only queries the persisted provider ID.
	input.ReferenceImages = []providerMedia{{StorageKey: "deleted-or-expired-reference"}}
	resume := context.WithValue(withProviderAnalytics(context.Background(), restarted, *claimed), mediaExecutionTaskKey{}, *claimed)
	result, err = runImageTask(resume, input)
	if err != nil {
		t.Fatal(err)
	}
	latest, err := restarted.repo.Task(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	claimed.MediaRecoveryJSON, claimed.MediaStage = latest.MediaRecoveryJSON, latest.MediaStage
	if err := restarted.finishTaskMediaRecovery(claimed, result, nil); err != nil {
		t.Fatal(err)
	}
	final, err := restarted.repo.Task(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != model.TaskStatusSucceeded || final.MediaStage != "completed" || final.ProviderRequestID != "durable-qwen" {
		t.Fatalf("not persisted: %+v", final)
	}
	if posts.Load() != 1 || polls.Load() != 2 || downloads.Load() != 1 {
		t.Fatalf("duplicate upstream work: %d %d %d", posts.Load(), polls.Load(), downloads.Load())
	}
	var count int64
	db.Model(&model.BillingOrder{}).Count(&count)
	if count != 0 {
		t.Fatal("recovery created an extra billing order")
	}
	db.Model(&model.Resource{}).Where("user_id = ?", task.UserID).Count(&count)
	if count != 1 {
		t.Fatalf("persisted resources: %d", count)
	}
}

func TestQwenAsyncPollErrorsAreBoundedAndFailuresNotSwallowed(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	for _, tc := range []struct {
		name  string
		code  int
		body  string
		calls int
	}{
		{"missing", 404, `{}`, 3}, {"server", 503, `{}`, 3}, {"invalid", 422, `{}`, 1},
		{"auth", 401, `{}`, 1}, {"failed", 200, `{"status":"error","error":"synthetic failure"}`, 1},
		{"unknown", 200, `{"status":"mystery"}`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodGet {
					t.Error("unexpected submit")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			policy := qwenImagePollPolicy{MaxTransientErrors: 3, Sleep: func(context.Context, time.Duration) error { return nil }}
			result, err := pollQwenImageTask(context.Background(), qwenTestInput(server.URL).Config, "existing", policy)
			if err == nil || result != nil || calls != tc.calls {
				t.Fatalf("result=%v err=%v calls=%d", result, err, calls)
			}
		})
	}
}

func TestQwenAsyncOnlyExactModelCanDefer(t *testing.T) {
	svc := &Service{}
	started := time.Now()
	task := model.Task{ID: "qwen", Type: "canvas_image", ProviderRequestID: "existing", StartedAt: &started}
	input := qwenTestInput("https://93.184.216.34")
	for _, name := range []string{"qwen-image-2.1", "gpt-image-1", "qwen-other"} {
		input.Config.Model = name
		encoded, _ := json.Marshal(input)
		if got := svc.shouldDeferProviderTask(task, string(encoded), context.DeadlineExceeded); got != (name == "qwen-image-2.1") {
			t.Fatalf("model %s defer=%v", name, got)
		}
	}
	input.Config.Model = "qwen-image-2.1"
	encoded, _ := json.Marshal(input)
	for _, failure := range []error{context.Canceled, providerHTTPError{StatusCode: 404}, errors.New("failed")} {
		if svc.shouldDeferProviderTask(task, string(encoded), failure) {
			t.Fatal("terminal error deferred")
		}
	}
	pending := providerStatePendingError{TaskID: "existing", Cause: providerHTTPError{StatusCode: 503}}
	if !svc.shouldDeferProviderTask(task, string(encoded), pending) {
		t.Fatal("temporary outage must query same task later")
	}
	expired := time.Now().Add(-qwenImageTaskMaxAge - time.Second)
	task.StartedAt = &expired
	if svc.shouldDeferProviderTask(task, string(encoded), pending) || svc.shouldDeferProviderTask(task, string(encoded), context.DeadlineExceeded) {
		t.Fatal("Qwen retry exceeded one-hour task lifetime")
	}
	task.StartedAt = &started
	task.ProviderRequestID = ""
	if svc.shouldDeferProviderTask(task, string(encoded), context.DeadlineExceeded) {
		t.Fatal("unknown submit must not repeat")
	}
}

func TestQwenAsyncSubmissionDisconnectIsNotRetried(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	posts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts++
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		connection.Close()
	}))
	defer server.Close()
	result, err := runImageTask(context.Background(), qwenTestInput(server.URL))
	if err == nil || result != nil || posts != 1 || !strings.Contains(err.Error(), "连接") {
		t.Fatalf("result=%v err=%v posts=%d", result, err, posts)
	}
}

func TestQwenAsyncOutageDefersThenResumesWithoutSubmit(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	png := mediaTestPNG(t)
	posts, polls, downloads := 0, 0, 0
	ready := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
			http.Error(w, "unexpected submit", 500)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/content") {
			downloads++
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
			return
		}
		polls++
		w.Header().Set("Content-Type", "application/json")
		if !ready {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"original-id","status":"done"}`))
	}))
	defer server.Close()
	input := qwenTestInput(server.URL)
	policy := qwenImagePollPolicy{MaxTransientErrors: 3, Sleep: func(context.Context, time.Duration) error { return nil }}
	result, err := pollQwenImageTask(context.Background(), input.Config, "original-id", policy)
	var pending providerStatePendingError
	if result != nil || !errors.As(err, &pending) || pending.TaskID != "original-id" || polls != 3 {
		t.Fatalf("not deferred safely: %v %v %d", result, err, polls)
	}
	ready = true
	ctx := context.WithValue(context.Background(), providerAnalyticsKey{}, providerAnalyticsContext{ProviderRequestID: pending.TaskID})
	result, err = runImageTask(ctx, input)
	if err != nil || result["mode"] != "image" || posts != 0 || polls != 4 || downloads != 1 {
		t.Fatalf("bad resume: %v %v post=%d poll=%d get=%d", result, err, posts, polls, downloads)
	}
}

func TestQwenAsyncContent404UsesDeliveryRecoveryNotGeneration(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	fixture, db := newMediaRecoveryTestService(t)
	s := New(fixture.repo, fixture.dataDir)
	png := mediaTestPNG(t)
	var posts, polls, downloads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			posts.Add(1)
			http.Error(w, "unexpected submit", 500)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/content") {
			if downloads.Add(1) == 1 {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
			return
		}
		polls.Add(1)
		_, _ = w.Write([]byte(`{"id":"content-race","status":"done"}`))
	}))
	defer server.Close()
	input := qwenTestInput(server.URL)
	task := seedMediaTask(t, db, input.Config)
	started := time.Now()
	if err := db.Model(task).Updates(map[string]any{"started_at": started, "provider_request_id": "content-race"}).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(withProviderAnalytics(context.Background(), s, *task), mediaExecutionTaskKey{}, *task)
	result, err := runImageTask(ctx, input)
	var delivery *mediaRecoveryError
	if result != nil || !errors.As(err, &delivery) || !delivery.retryable {
		t.Fatalf("content race not recoverable: %v %v", result, err)
	}
	if err := s.handleMediaRecoveryFailure(task, delivery); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(task).Update("next_poll_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	restarted := New(s.repo, s.dataDir)
	claimed, err := restarted.repo.ClaimNextTask("delivery-after-restart", time.Minute)
	if err != nil || claimed == nil {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	if err := restarted.taskWorker().processClaimedTask(claimed, nil); err != nil {
		t.Fatal(err)
	}
	final, err := restarted.repo.Task(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != model.TaskStatusSucceeded || final.MediaStage != "completed" || posts.Load() != 0 || polls.Load() != 1 || downloads.Load() != 2 {
		t.Fatalf("bad delivery recovery: status=%s stage=%s post=%d poll=%d get=%d", final.Status, final.MediaStage, posts.Load(), polls.Load(), downloads.Load())
	}
}

func TestQwenAsyncCheckpointMustExistBeforePolling(t *testing.T) {
	s, _ := newMediaRecoveryTestService(t)
	ctx := withProviderAnalytics(context.Background(), s, model.Task{ID: "missing-task", Type: "canvas_image"})
	result, err := finishQwenImageCreate(ctx, providerConfig{}, map[string]any{"id": "accepted-but-unsaved", "status": "queued"})
	if result != nil || err == nil || !strings.Contains(err.Error(), "确认 Qwen") {
		t.Fatalf("unsafe checkpoint: %v %v", result, err)
	}
}

func TestQwenAsyncRealConnectionRefusalResumesOriginalID(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	stopped := httptest.NewServer(http.NotFoundHandler())
	address, baseURL := stopped.Listener.Addr().String(), stopped.URL
	stopped.Close()
	input := qwenTestInput(baseURL)
	policy := qwenImagePollPolicy{MaxTransientErrors: 2, Sleep: func(context.Context, time.Duration) error { return nil }}
	result, err := pollQwenImageTask(context.Background(), input.Config, "restart-id", policy)
	var pending providerStatePendingError
	if result != nil || !errors.As(err, &pending) || !errors.Is(err, syscall.ECONNREFUSED) || pending.TaskID != "restart-id" {
		t.Fatalf("real refusal not deferred: %v %v", result, err)
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	png := mediaTestPNG(t)
	posts, polls, downloads := 0, 0, 0
	restarted := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
			http.Error(w, "no new generation", 500)
			return
		}
		if r.URL.Path == "/v1/images/restart-id/content" {
			downloads++
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
			return
		}
		polls++
		if r.URL.Path != "/v1/images/restart-id" {
			t.Error(r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"restart-id","status":"done"}`))
	}))
	restarted.Listener.Close()
	restarted.Listener = listener
	restarted.Start()
	defer restarted.Close()
	ctx := context.WithValue(context.Background(), providerAnalyticsKey{}, providerAnalyticsContext{ProviderRequestID: pending.TaskID})
	result, err = runImageTask(ctx, input)
	if err != nil || result["mode"] != "image" || posts != 0 || polls != 1 || downloads != 1 {
		t.Fatalf("resume: %v %v posts=%d polls=%d downloads=%d", result, err, posts, polls, downloads)
	}
}

func TestQwenAsyncRealConnectionResetRetriesGETOnly(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	png := mediaTestPNG(t)
	posts, polls, downloads := 0, 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
			http.Error(w, "no new generation", 500)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/content") {
			downloads++
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
			return
		}
		polls++
		if polls == 1 {
			connection, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			if tcp, ok := connection.(*net.TCPConn); ok {
				_ = tcp.SetLinger(0)
			}
			connection.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"reset-id","status":"done"}`))
	}))
	defer server.Close()
	policy := qwenImagePollPolicy{MaxTransientErrors: 3, Sleep: func(context.Context, time.Duration) error { return nil }}
	result, err := pollQwenImageTask(context.Background(), qwenTestInput(server.URL).Config, "reset-id", policy)
	if err != nil || result["mode"] != "image" || posts != 0 || polls != 2 || downloads != 1 {
		t.Fatalf("reset recovery: %v %v %d %d %d", result, err, posts, polls, downloads)
	}
}

func TestQwenAsyncConnectionRetryPolicyIsNarrow(t *testing.T) {
	config := qwenTestInput("https://example.invalid").Config
	for _, cause := range []error{syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.EPIPE} {
		err := &net.OpError{Op: "read", Net: "tcp", Err: fmt.Errorf("wrapped: %w", cause)}
		if !retryableQwenImageConnectionError(err) || !retryableQwenImageContentError(config, err) {
			t.Fatalf("socket error not retryable: %v", err)
		}
		other := config
		other.Model = "gpt-image-1"
		if retryableQwenImageContentError(other, err) {
			t.Fatal("other provider retry policy changed")
		}
	}
	for _, err := range []error{x509.UnknownAuthorityError{}, syscall.EACCES, &net.DNSError{Err: "no such host", Name: "example.invalid"}, providerHTTPError{StatusCode: 401}, providerHTTPError{StatusCode: 422}, errors.New("invalid URL")} {
		if retryableQwenImageConnectionError(err) || retryableQwenImageContentError(config, err) {
			t.Fatalf("permanent error masked: %v", err)
		}
	}
}

func TestQwenAsyncRealContentConnectionRefusalRecoversDownloadOnly(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	fixture, db := newMediaRecoveryTestService(t)
	s := New(fixture.repo, fixture.dataDir)
	var posts, polls, downloads atomic.Int32
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			posts.Add(1)
			http.Error(w, "no submit", 500)
			return
		}
		polls.Add(1)
		// Publish done, then simulate the API restarting before /content.
		w.Header().Set("Connection", "close")
		w.Header().Set("Content-Type", "application/json")
		_ = upstream.Listener.Close()
		_, _ = w.Write([]byte(`{"id":"content-restart","status":"done"}`))
	}))
	defer upstream.Close()
	address := upstream.Listener.Addr().String()
	input := qwenTestInput(upstream.URL)
	task := seedMediaTask(t, db, input.Config)
	if err := db.Model(task).Updates(map[string]any{"started_at": time.Now(), "provider_request_id": "content-restart"}).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(withProviderAnalytics(context.Background(), s, *task), mediaExecutionTaskKey{}, *task)
	result, err := runImageTask(ctx, input)
	var delivery *mediaRecoveryError
	if result != nil || !errors.As(err, &delivery) || !delivery.retryable || !errors.Is(err, syscall.ECONNREFUSED) {
		t.Fatalf("content refusal not recoverable: %v %v", result, err)
	}
	if err := s.handleMediaRecoveryFailure(task, delivery); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	png := mediaTestPNG(t)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			posts.Add(1)
			http.Error(w, "no submit", 500)
			return
		}
		if r.URL.Path != "/v1/images/content-restart/content" {
			polls.Add(1)
			http.Error(w, "download only", 500)
			return
		}
		downloads.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	defer server.Close()
	if err := db.Model(task).Update("next_poll_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	restarted := New(s.repo, s.dataDir)
	claimed, err := restarted.repo.ClaimNextTask("content-restarted", time.Minute)
	if err != nil || claimed == nil {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	if err := restarted.taskWorker().processClaimedTask(claimed, nil); err != nil {
		t.Fatal(err)
	}
	final, err := restarted.repo.Task(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != model.TaskStatusSucceeded || posts.Load() != 0 || polls.Load() != 1 || downloads.Load() != 1 {
		t.Fatalf("bad recovery: %s %d %d %d", final.Status, posts.Load(), polls.Load(), downloads.Load())
	}
}
