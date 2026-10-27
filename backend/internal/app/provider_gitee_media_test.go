package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"yingce/backend/internal/model"
)

type litterboxTestTransport func(*http.Request) (*http.Response, error)

func (f litterboxTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestLitterboxUploadContract(t *testing.T) {
	client := &http.Client{Transport: litterboxTestTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != litterboxEndpoint || r.Header.Get("Authorization") != "" {
			t.Fatal("unexpected endpoint or credential leak")
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		defer r.MultipartForm.RemoveAll()
		if r.FormValue("time") != "12h" || r.FormValue("reqtype") != "fileupload" {
			t.Fatal("incorrect retention")
		}
		file, header, err := r.FormFile("fileToUpload")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		body, _ := io.ReadAll(file)
		if string(body) != "image-data" || header.Filename != "reference.png" {
			t.Fatal("incorrect file")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("https://litter.catbox.moe/example.png\n"))}, nil
	})}
	link, err := uploadLitterbox(context.Background(), client, []byte("image-data"), "image/png")
	if err != nil || link != "https://litter.catbox.moe/example.png" {
		t.Fatalf("%s %v", link, err)
	}
}

func TestLitterboxRejectsInvalidResponse(t *testing.T) {
	for _, raw := range []string{"http://litter.catbox.moe/x.png", "https://evil.example/x.png", "https://litter.catbox.moe@evil.example/x.png", "error uploading", "https://litter.catbox.moe/x.png?token=secret"} {
		t.Run(raw, func(t *testing.T) {
			client := &http.Client{Transport: litterboxTestTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(raw))}, nil
			})}
			if _, err := uploadLitterbox(context.Background(), client, []byte("x"), "image/png"); err == nil {
				t.Fatal("accepted invalid response")
			}
		})
	}
}

func TestGiteeMediaRequiresConsent(t *testing.T) {
	var svc Service
	in := canvasGenerationInput{Config: providerConfig{InterfaceType: giteeH3Protocol}}
	if err := svc.prepareGiteeMedia(context.Background(), "user", &in); err == nil {
		t.Fatal("missing consent accepted")
	}
	in.Config.InterfaceType = "minimax-video"
	if err := svc.prepareGiteeMedia(context.Background(), "user", &in); err != nil {
		t.Fatal("local H3 changed")
	}
}

func TestGiteeCatalogWithoutResolutionControl(t *testing.T) {
	raw, err := os.ReadFile("../../../deploy/gitee-h3/model.json")
	if err != nil {
		t.Fatal(err)
	}
	var request ChannelModelRequest
	if err = json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	spec, err := CapabilitySpecFromModelCapabilityConfig(request.CapabilityConfig, "video")
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(request.CapabilityConfig)
	defaults, err := channelModelDefaultOptions(model.ChannelModel{ModelKey: request.ModelKey, ProviderModelKey: request.ProviderModelKey, Protocol: model.ChannelInterfaceType(request.Protocol), Capability: "video", CapabilityConfigJSON: string(encoded)}, spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := defaults["vquality"]; ok {
		t.Fatal("unsupported resolution added")
	}
}

func TestGiteeTextSkipsPublicUpload(t *testing.T) {
	var svc Service
	input := canvasGenerationInput{Config: providerConfig{InterfaceType: giteeH3Protocol}, Metadata: map[string]any{"videoEditOperation": "text_to_video"}}
	if err := svc.prepareGiteeMedia(context.Background(), "user", &input); err != nil {
		t.Fatal(err)
	}
	input.ReferenceImages = []providerMedia{{ID: "a"}}
	if err := svc.prepareGiteeMedia(context.Background(), "user", &input); err == nil {
		t.Fatal("text task accepted a reference")
	}
}

func TestGiteeModeValidationBeforeUpload(t *testing.T) {
	for _, tc := range []struct {
		op     string
		images []providerMedia
		bad    bool
	}{
		{"image_to_video", []providerMedia{{ID: "a", Role: "first_frame"}, {ID: "b", Role: "last_frame"}}, false},
		{"image_to_video", []providerMedia{{ID: "a", Role: "first_frame"}, {ID: "b", Role: "first_frame"}}, true},
		{"image_to_video", []providerMedia{{ID: "a"}, {ID: "b"}}, true},
		{"reference_to_video", []providerMedia{{ID: "a", Role: "last_frame"}}, true},
		{"reference_to_video", nil, true},
	} {
		in := canvasGenerationInput{Metadata: map[string]any{"videoEditOperation": tc.op}, ReferenceImages: tc.images}
		err := validateGiteeMode(&in)
		if (err != nil) != tc.bad {
			t.Fatalf("%s: %v", tc.op, err)
		}
	}
}
