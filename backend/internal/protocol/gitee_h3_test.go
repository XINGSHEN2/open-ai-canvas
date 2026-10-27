package protocol

import (
	"context"
	"os"
	"testing"
)

func TestGiteeH3Contract(t *testing.T) {
	raw, err := os.ReadFile("../../../deploy/gitee-h3/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	adapters, err := LoadInstalledProviders(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := adapters[0]
	spec, err := a.BuildCreate(context.Background(), RequestContext{Request: GenerationRequest{Model: "MiniMax-H3", Prompt: "test", Operation: "reference_to_video", Duration: 6, AspectRatio: "16:9", Images: []MediaReference{{URL: "https://litter.catbox.moe/two.png", Order: 2}, {URL: "https://litter.catbox.moe/one.png", Order: 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	body := manifestTestBody(t, spec)
	if body["num_steps"] != float64(20) || body["task"] != "ref2va" || spec.Path != "/v1/async/videos/generations" {
		t.Fatalf("%#v", body)
	}
	imgs := body["cond_image"].([]any)
	if imgs[0] != "https://litter.catbox.moe/one.png" {
		t.Fatal("reference order changed")
	}
	if _, ok := body["resolution"]; ok {
		t.Fatal("unsupported resolution leaked")
	}
	poll, err := a.BuildPoll(context.Background(), PollContext{TaskID: "abc"})
	if err != nil || poll.Path != "/api/v1/task/abc" {
		t.Fatalf("%#v %v", poll, err)
	}
	for _, tc := range []struct {
		raw    string
		status Status
	}{
		{`{"task_id":"abc","status":"waiting"}`, StatusPending},
		{`{"task_id":"abc","status":"in_progress"}`, StatusProcessing},
		{`{"task_id":"abc","status":"success","output":{"file_url":"https://cdn.example/out.mp4"}}`, StatusSucceeded},
		{`{"task_id":"abc","status":"failed","error":"download failed"}`, StatusFailed},
		{`{"task_id":"abc","status":"cancelled"}`, StatusCancelled},
	} {
		result, err := a.ParsePoll(context.Background(), PollContext{TaskID: "abc"}, []byte(tc.raw))
		if err != nil || result.Status != tc.status {
			t.Fatalf("%#v %v", result, err)
		}
	}
}

func TestGiteeH3Modes(t *testing.T) {
	raw, err := os.ReadFile("../../../deploy/gitee-h3/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	adapters, err := LoadInstalledProviders(raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, op, task string
		images         []MediaReference
		bad            bool
	}{
		{"text", "text_to_video", "t2va", nil, false},
		{"text rejects images", "text_to_video", "", []MediaReference{{URL: "https://test/image.png"}}, true},
		{"reversed frame order", "image_to_video", "fl2va", []MediaReference{{URL: "https://test/last.png", Role: "last_frame"}, {URL: "https://test/first.png", Role: "first_frame"}}, false},
		{"missing last", "image_to_video", "", []MediaReference{{URL: "https://test/first.png", Role: "first_frame"}}, true},
		{"duplicate first", "image_to_video", "", []MediaReference{{URL: "https://test/a.png", Role: "first_frame"}, {URL: "https://test/b.png", Role: "first_frame"}}, true},
		{"unlabeled frames", "image_to_video", "", []MediaReference{{URL: "https://test/a.png"}, {URL: "https://test/b.png"}}, true},
		{"empty refs", "reference_to_video", "", nil, true},
		{"unknown", "extend", "", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := adapters[0].BuildCreate(context.Background(), RequestContext{Request: GenerationRequest{Model: "MiniMax-H3", Prompt: "test", Duration: 6, AspectRatio: "16:9", Operation: tc.op, Images: tc.images}})
			if tc.bad {
				if err == nil {
					t.Fatal("invalid input accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			body := manifestTestBody(t, spec)
			if body["task"] != tc.task {
				t.Fatalf("%#v", body)
			}
			for _, key := range []string{"cond_image", "cond_video", "cond_audio"} {
				if _, ok := body[key]; ok {
					t.Fatal("reference fields leaked", key)
				}
			}
			if tc.task == "fl2va" {
				if body["first_frame"] != "https://test/first.png" || body["last_frame"] != "https://test/last.png" {
					t.Fatalf("frame roles swapped: %#v", body)
				}
			} else {
				if _, ok := body["first_frame"]; ok {
					t.Fatal("frame field in text task")
				}
			}
		})
	}
}
