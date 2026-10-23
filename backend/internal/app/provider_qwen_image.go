package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"yingce/backend/internal/model"
	"yingce/backend/internal/protocol"
)

func isLocalQwenImageConfig(config providerConfig) bool {
	return strings.TrimSpace(config.InterfaceType) == string(model.ChannelInterfaceOpenAIImage) && strings.TrimSpace(config.Model) == "qwen-image-2.1"
}

const qwenImageTaskMaxAge = time.Hour

func qwenImageTaskCanResume(task model.Task, now time.Time) bool {
	return task.StartedAt != nil && now.Before(task.StartedAt.Add(qwenImageTaskMaxAge))
}

func retryableQwenImageContentError(config providerConfig, err error) bool {
	var upstream providerHTTPError
	return isLocalQwenImageConfig(config) && (retryableQwenImageConnectionError(err) ||
		(errors.As(err, &upstream) && upstream.StatusCode == http.StatusNotFound))
}

// Only GET status/content calls use this predicate. A refused or reset socket
// after an accepted task is recoverable; a failed create remains ambiguous and
// must never be repeated. Do not classify TLS, DNS, URL or permission errors by
// broad net.Error/string matches.
func retryableQwenImageConnectionError(err error) bool {
	for _, cause := range []error{syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.ECONNABORTED,
		syscall.EPIPE, syscall.ENETUNREACH, syscall.EHOSTUNREACH, io.EOF, io.ErrUnexpectedEOF, io.ErrClosedPipe} {
		if errors.Is(err, cause) {
			return true
		}
	}
	return false
}

type qwenImagePollPolicy struct {
	Interval           time.Duration
	MaxTransientErrors int
	Sleep              func(context.Context, time.Duration) error
}

func defaultQwenImagePollPolicy() qwenImagePollPolicy {
	return qwenImagePollPolicy{Interval: 3 * time.Second, MaxTransientErrors: 5, Sleep: sleepContext}
}

func finishQwenImageCreate(ctx context.Context, config providerConfig, payload map[string]interface{}) (map[string]interface{}, error) {
	// Older deployments may ignore Prefer and finish synchronously. Only an
	// actual OpenAI image result is success; an accepted ID is not an image.
	if _, ok := payload["data"]; ok {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		var response imageResponse
		if err := json.Unmarshal(data, &response); err != nil {
			return nil, err
		}
		images, err := imageDataURLs(response)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"mode": "image", "images": images}, nil
	}
	taskID := firstNonEmpty(stringField(payload, "task_id"), stringField(payload, "id"))
	if err := validateQwenImageTaskID(taskID); err != nil {
		return nil, err
	}
	metadata, ok := ctx.Value(providerAnalyticsKey{}).(providerAnalyticsContext)
	if ok && metadata.Service != nil {
		// Unlike best-effort API logging, this checkpoint must succeed before
		// polling. Worker recovery will only GET this ID, never repeat the POST.
		if err := metadata.Service.recordWorkflowProviderRequest(ctx, taskID, "accepted", nil); err != nil {
			return nil, fmt.Errorf("保存 Qwen 上游任务 ID 失败，已停止继续提交：%w", err)
		}
		saved, err := metadata.Service.repo.Task(metadata.TaskID)
		if err != nil {
			return nil, fmt.Errorf("确认 Qwen 上游任务 ID 失败：%w", err)
		}
		if saved.ProviderRequestID != taskID {
			return nil, errors.New("Qwen 上游任务 ID 未持久化，已停止继续提交")
		}
	}
	metadata.ProviderRequestID = taskID
	ctx = context.WithValue(ctx, providerAnalyticsKey{}, metadata)
	return pollQwenImageTask(ctx, config, taskID, defaultQwenImagePollPolicy())
}

func validateQwenImageTaskID(taskID string) error {
	if strings.TrimSpace(taskID) == "" || len(taskID) > 256 || strings.ContainsAny(taskID, "/\\?#\r\n") || taskID == "." || taskID == ".." {
		return errors.New("Qwen 图片接口没有返回有效任务 ID")
	}
	return nil
}

func pollQwenImageTask(ctx context.Context, config providerConfig, taskID string, policy qwenImagePollPolicy) (map[string]interface{}, error) {
	if err := validateQwenImageTaskID(taskID); err != nil {
		return nil, err
	}
	deadline := providerPollingDeadline(ctx)
	if metadata, ok := ctx.Value(providerAnalyticsKey{}).(providerAnalyticsContext); ok && metadata.Service != nil && metadata.TaskID != "" {
		task, err := metadata.Service.repo.Task(metadata.TaskID)
		if err != nil {
			return nil, err
		}
		if task.StartedAt == nil {
			return nil, errors.New("Qwen 图片任务缺少开始时间，无法安全恢复")
		}
		if limit := task.StartedAt.Add(qwenImageTaskMaxAge); limit.Before(deadline) {
			deadline = limit
		}
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	path := "/images/" + url.PathEscape(taskID)
	consecutiveErrors := 0
	for {
		var payload map[string]interface{}
		err := getJSON(withProviderRequestKind(ctx, "poll"), config, path, &payload)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			consecutiveErrors++
			retry, _ := retryableVideoPollError(ctx, err)
			if retryableQwenImageConnectionError(err) {
				retry = true
			}
			// Bound every transient failure, including 404 and 5xx. A permanent
			// status/parameter error must not be converted into endless polling.
			if !retry {
				return nil, err
			}
			if consecutiveErrors >= policy.MaxTransientErrors {
				return nil, providerStatePendingError{TaskID: taskID, Cause: err}
			}
		} else {
			consecutiveErrors = 0
			if returned := firstNonEmpty(stringField(payload, "task_id"), stringField(payload, "id")); returned != "" && returned != taskID {
				return nil, errors.New("Qwen 返回了不匹配的图片任务 ID")
			}
			switch strings.ToLower(strings.TrimSpace(stringField(payload, "status"))) {
			case "done", "completed", "succeeded", "success":
				// Checkpoint the same-origin content URL before downloading. Existing
				// media recovery persists the file and retries delivery without a new job.
				reference := protocol.MediaReference{URL: apiURL(config.BaseURL, path+"/content"), MIMEType: "image/png"}
				if result, handled, err := recoverProtocolMedia(ctx, config, "image", []protocol.MediaReference{reference}); handled {
					return result, err
				}
				data, mimeType, err := getBinary(withProviderRequestKind(ctx, "download"), config, path+"/content")
				if err != nil {
					return nil, err
				}
				if len(data) == 0 || !strings.HasPrefix(http.DetectContentType(data), "image/") {
					return nil, errors.New("Qwen 图片任务已完成但下载内容不是图片")
				}
				return map[string]interface{}{"mode": "image", "images": []map[string]string{{"dataUrl": dataURL(normalizedMediaMimeType(mimeType, data), data)}}}, nil
			case "error", "failed", "cancelled", "canceled":
				return nil, errors.New("Qwen 图片生成失败或已取消，请在任务中心查看详情")
			case "queued", "pending", "submitted", "running", "processing":
			default:
				return nil, errors.New("Qwen 图片接口返回未知任务状态")
			}
		}
		if err := policy.Sleep(ctx, max(policy.Interval, providerRetryAfter(err))); err != nil {
			return nil, err
		}
	}
}
