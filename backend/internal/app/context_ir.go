package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"yingce/backend/internal/assets"
	"yingce/backend/internal/outbound"
)

type IRAsset struct {
	ResourceID string `json:"resourceId"`
	Kind       string `json:"kind"`
	Label      string `json:"label"`
}
type IRPromptRequest struct {
	Prompt     string    `json:"prompt"`
	Assets     []IRAsset `json:"assets"`
	Seconds    int       `json:"seconds"`
	Ratio      string    `json:"ratio"`
	Resolution string    `json:"resolution"`
}
type IRPromptJob struct {
	ID         string     `json:"id"`
	Status     string     `json:"status"`
	Prompt     string     `json:"prompt,omitempty"`
	Error      string     `json:"error,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

// Single-instance adapter. Disk results survive restart; interrupted jobs are never resubmitted.
var irJobs = struct {
	sync.Mutex
	active map[string]bool
}{active: make(map[string]bool)}

func validateIRRequest(req IRPromptRequest) error {
	if strings.TrimSpace(req.Prompt) == "" || len(req.Prompt) > 100000 {
		return BadAuthRequest("请填写提示词（最多 100000 字节）")
	}
	if req.Seconds < 5 || req.Seconds > 15 {
		return BadAuthRequest("IR 视频时长须为 5–15 秒")
	}
	if req.Ratio != "16:9" && req.Ratio != "9:16" {
		return BadAuthRequest("IR 仅支持 16:9 或 9:16")
	}
	if req.Resolution != "480" && req.Resolution != "768" {
		return BadAuthRequest("IR 仅支持 480P 或 768P")
	}
	if len(req.Assets) > 15 {
		return BadAuthRequest("参考素材过多")
	}
	counts := map[string]int{}
	for _, a := range req.Assets {
		if a.ResourceID == "" || len(a.Label) > 256 || (a.Kind != "image" && a.Kind != "video" && a.Kind != "audio") {
			return BadAuthRequest("参考素材无效")
		}
		counts[a.Kind]++
	}
	if counts["image"] > 9 || counts["video"] > 3 || counts["audio"] > 3 {
		return BadAuthRequest("参考素材超出 H3 输入上限")
	}
	return nil
}

func (s *Service) irJobPath(userID, id string) string {
	sum := sha256.Sum256([]byte(userID))
	return filepath.Join(s.dataDir, "context-ir-jobs", hex.EncodeToString(sum[:]), id+".json")
}
func saveIRJob(path string, job IRPromptJob) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	body, err := json.Marshal(job)
	if err != nil {
		return err
	}
	if err = os.WriteFile(path+".tmp", body, 0600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

func (s *Service) CreateIRPrompt(userID string, req IRPromptRequest) (*IRPromptJob, error) {
	if userID == "" {
		return nil, Unauthorized("请先登录")
	}
	if err := validateIRRequest(req); err != nil {
		return nil, err
	}
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("CANVAS_CONTEXT_IR_BASE_URL")), "/")
	if base == "" {
		return nil, WrapAppError(503, "IR 服务尚未配置", nil)
	}
	endpoint, err := outbound.ValidateOutboundURL(base + "/api/h3/prompt")
	if err != nil {
		return nil, err
	}
	refs := []map[string]any{}
	counts := map[string]int{}
	for _, a := range req.Assets {
		resource, err := s.Resource(userID, a.ResourceID)
		if err != nil || resource == nil || resource.UserID != userID {
			return nil, NotFound("参考素材不存在或不可访问")
		}
		if resource.Kind != a.Kind {
			return nil, BadAuthRequest("参考素材类型不匹配")
		}
		uri, err := s.signedResourceAccessURL(resource, assets.VariantOriginal, time.Now().Add(2*time.Hour), true)
		if err != nil {
			return nil, err
		}
		counts[a.Kind]++
		refs = append(refs, map[string]any{"asset_id": fmt.Sprintf("%s_%d", a.Kind, counts[a.Kind]), "media_type": a.Kind, "uri": uri, "label": a.Label, "user_role": "reference"})
	}
	task := "ref2va"
	if len(refs) == 0 {
		task = "t2va"
	}
	body, err := json.Marshal(map[string]any{"input_type": "assets", "source": map[string]any{
		"schema_version": "context_request.v1", "user_request": req.Prompt, "assets": refs,
		"task": map[string]any{"type": task, "duration_seconds": req.Seconds, "aspect_ratio": req.Ratio},
	}})
	if err != nil {
		return nil, err
	}
	job := IRPromptJob{ID: uuid.NewString(), Status: "running", CreatedAt: time.Now().UTC()}
	path := s.irJobPath(userID, job.ID)
	irJobs.Lock()
	if len(irJobs.active) >= 2 {
		irJobs.Unlock()
		return nil, WrapAppError(429, "IR 正在处理其他请求，请稍后重试", nil)
	}
	irJobs.active[path] = true
	irJobs.Unlock()
	if err := saveIRJob(path, job); err != nil {
		irJobs.Lock()
		delete(irJobs.active, path)
		irJobs.Unlock()
		return nil, err
	}
	go func(job IRPromptJob) {
		defer func() { irJobs.Lock(); delete(irJobs.active, path); irJobs.Unlock() }()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		prompt, err := callIRPrompt(ctx, endpoint.String(), body)
		now := time.Now().UTC()
		job.FinishedAt = &now
		if err != nil {
			job.Status = "failed"
			job.Error = "IR 优化失败或超时，请检查 IR 服务后重试；原提示词未修改"
		} else {
			job.Status = "completed"
			job.Prompt = prompt
		}
		// A failed final write leaves a running record; querying it reports interruption.
		_ = saveIRJob(path, job)
	}(job)
	return &job, nil
}

func callIRPrompt(ctx context.Context, endpoint string, body []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := outbound.OutboundHTTPClient(30 * time.Minute).Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("IR status %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	if err != nil {
		return "", err
	}
	if len(raw) > 8<<20 {
		return "", fmt.Errorf("IR response too large")
	}
	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			Prompt string `json:"h3_prompt"`
		} `json:"result"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return "", err
	}
	if !result.OK || strings.TrimSpace(result.Result.Prompt) == "" {
		return "", fmt.Errorf("IR missing prompt")
	}
	return result.Result.Prompt, nil
}

func (s *Service) IRPrompt(userID, id string) (*IRPromptJob, error) {
	if userID == "" {
		return nil, Unauthorized("请先登录")
	}
	if _, err := uuid.Parse(id); err != nil {
		return nil, BadAuthRequest("IR 任务 ID 无效")
	}
	path := s.irJobPath(userID, id)
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, NotFound("IR 任务不存在或不可访问")
	}
	if err != nil {
		return nil, err
	}
	var job IRPromptJob
	if err = json.Unmarshal(raw, &job); err != nil {
		return nil, err
	}
	irJobs.Lock()
	active := irJobs.active[path]
	irJobs.Unlock()
	if job.Status == "running" && !active {
		job.Status = "failed"
		job.Error = "IR 任务已中断，请重新优化"
	}
	return &job, nil
}
