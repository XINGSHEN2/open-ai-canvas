package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestContextIRValidation(t *testing.T) {
	valid := IRPromptRequest{Prompt: "test", Seconds: 15, Ratio: "9:16", Resolution: "480"}
	if err := validateIRRequest(valid); err != nil {
		t.Fatal(err)
	}
	cases := []IRPromptRequest{valid, valid, valid, valid}
	cases[0].Prompt = " "
	cases[1].Ratio = "1:1"
	cases[2].Resolution = "1080"
	cases[3].Seconds = 16
	for _, req := range cases {
		if validateIRRequest(req) == nil {
			t.Fatalf("accepted invalid input: %#v", req)
		}
	}
	for i := 0; i < 8; i++ {
		valid.Assets = append(valid.Assets, IRAsset{ResourceID: "image", Kind: "image"})
	}
	if err := validateIRRequest(valid); err != nil {
		t.Fatal(err)
	}
	valid.Assets = append(valid.Assets, IRAsset{ResourceID: "ninth", Kind: "image"})
	if validateIRRequest(valid) == nil {
		t.Fatal("IR must reject nine images before contacting upstream")
	}
}

func TestContextIRResultAndIsolation(t *testing.T) {
	svc := &Service{dataDir: t.TempDir()}
	id := uuid.NewString()
	job := IRPromptJob{ID: id, Status: "completed", Prompt: "edited prompt", CreatedAt: time.Now()}
	if err := saveIRJob(svc.irJobPath("owner", id), job); err != nil {
		t.Fatal(err)
	}
	got, err := svc.IRPrompt("owner", id)
	if err != nil || got.Prompt != job.Prompt {
		t.Fatalf("%v %v", got, err)
	}
	if _, err = svc.IRPrompt("other", id); err == nil {
		t.Fatal("cross-user read allowed")
	}
	if _, err = svc.IRPrompt("", id); err == nil {
		t.Fatal("anonymous read allowed")
	}
	if _, err = svc.IRPrompt("owner", "../../secret"); err == nil {
		t.Fatal("path traversal allowed")
	}
	job.Status = "running"
	if err = saveIRJob(svc.irJobPath("owner", id), job); err != nil {
		t.Fatal(err)
	}
	got, err = svc.IRPrompt("owner", id)
	if err != nil || got.Status != "failed" {
		t.Fatalf("interrupted job not detected: %v %v", got, err)
	}
}

func TestContextIRUpstreamResponse(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	for _, tc := range []struct {
		body string
		ok   bool
	}{
		{`{"ok":true,"result":{"h3_prompt":"optimized","h3_request":{"target":{"short_edge":768}}}}`, true},
		{`{"ok":false,"error":"private upstream detail"}`, false},
		{`{"ok":true,"result":{"h3_prompt":""}}`, false},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(tc.body))
		}))
		prompt, err := callIRPrompt(context.Background(), server.URL, []byte(`{}`))
		server.Close()
		if (err == nil) != tc.ok {
			t.Fatalf("unexpected response: %s %v", prompt, err)
		}
		if tc.ok && prompt != "optimized" {
			t.Fatal("incorrect prompt")
		}
	}
}

func TestContextIRRequestIDIsIdempotentAndRejectsChangedInput(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"h3_prompt":"optimized"}}`))
	}))
	defer server.Close()
	t.Setenv("CANVAS_CONTEXT_IR_BASE_URL", server.URL)
	service := &Service{dataDir: t.TempDir()}
	request := IRPromptRequest{RequestID: uuid.NewString(), Prompt: "test", Seconds: 5, Ratio: "16:9", Resolution: "480"}
	first, err := service.CreateIRPrompt("owner", request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateIRPrompt("owner", request)
	if err != nil || second.ID != first.ID {
		t.Fatalf("duplicate request: %v %v", second, err)
	}
	for i := 0; i < 200; i++ {
		job, err := service.IRPrompt("owner", first.ID)
		if err != nil {
			t.Fatal(err)
		}
		if job.Status == "completed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	result, err := service.IRPrompt("owner", first.ID)
	if err != nil || result.Status != "completed" || calls.Load() != 1 {
		t.Fatalf("status=%v error=%v calls=%d", result, err, calls.Load())
	}
	request.Prompt = "different"
	if _, err = service.CreateIRPrompt("owner", request); err == nil {
		t.Fatal("request ID reused for different input")
	}
}
