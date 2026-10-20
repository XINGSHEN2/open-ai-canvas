package app

import "testing"

func TestReferenceOnlyVideoRequiresMediaAndUsesReferenceOperation(t *testing.T) {
	profile := &VideoCapabilityConfig{References: VideoReferenceConfig{PromptMaxChars: 7000, MaxImages: 9, MaxVideos: 3, MaxAudios: 3}, Duration: VideoDurationConfig{Selection: "range", Min: 5, Max: 15, Step: 1, Default: 5}, Ratios: []string{"16:9"}, Resolutions: []string{"480P", "768P"}, Operations: []string{"reference_to_video"}, DefaultOperation: "reference_to_video"}
	input := canvasGenerationInput{Prompt: "cube", Config: providerConfig{VideoSeconds: "5", Size: "16:9", VQuality: "480P"}}
	if err := validateVideoTask(profile, input); err == nil {
		t.Fatal("empty reference task accepted")
	}
	input.ReferenceImages = []providerMedia{{URL: "https://example.com/image.png"}}
	if err := validateVideoTask(profile, input); err != nil {
		t.Fatal(err)
	}
	input.Config.VideoSeconds = "30"
	if err := validateVideoTask(profile, input); err == nil {
		t.Fatal("unsupported duration accepted")
	}
	input.Config.VideoSeconds = "5"
	input.Config.VQuality = "2K"
	if err := validateVideoTask(profile, input); err == nil {
		t.Fatal("unsupported resolution accepted")
	}
}
