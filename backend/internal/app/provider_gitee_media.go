package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"yingce/backend/internal/outbound"
)

const giteeH3Protocol = "gitee-h3-video"
const litterboxEndpoint = "https://litterbox.catbox.moe/resources/internals/api.php"
const litterboxMaxBytes int64 = 100 << 20

// Only explicitly consented, ownership-checked resources leave local storage.
// No shared URL cache: another user's references must never be reused or exposed.
func (s *Service) prepareGiteeMedia(ctx context.Context, userID string, input *canvasGenerationInput) error {
	if input.Config.InterfaceType != giteeH3Protocol {
		return nil
	}
	if err := validateGiteeMode(input); err != nil {
		return err
	}
	if metadataString(input.Metadata, "videoEditOperation") == "text_to_video" {
		return nil
	}
	consent, _ := input.Metadata["litterboxUploadConsent"].(bool)
	if !consent {
		return errors.New("使用 Gitee H3 前，请确认参考素材将上传至 Litterbox 公网临时托管 12 小时")
	}
	for _, group := range [][]providerMedia{input.ReferenceImages, input.ReferenceVideos, input.ReferenceAudios} {
		for i := range group {
			media := &group[i]
			if !strings.HasPrefix(media.StorageKey, "resource:") {
				return errors.New("Gitee H3 参考素材请先上传到本站素材库，再生成视频")
			}
			resource, body, err := s.OpenResource(userID, strings.TrimPrefix(media.StorageKey, "resource:"))
			if err != nil {
				return fmt.Errorf("读取参考素材失败：%w", err)
			}
			if resource.Size <= 0 || resource.Size > litterboxMaxBytes {
				body.Close()
				return errors.New("Gitee 中转素材必须非空且不超过 100 MiB")
			}
			content, readErr := io.ReadAll(io.LimitReader(body, litterboxMaxBytes+1))
			body.Close()
			if readErr != nil {
				return errors.New("读取参考素材内容失败")
			}
			if len(content) == 0 || int64(len(content)) > litterboxMaxBytes {
				return errors.New("参考素材大小超出中转限制")
			}
			mimeType := normalizedMediaMimeType(resource.MimeType, content)
			if !strings.HasPrefix(mimeType, "image/") && !strings.HasPrefix(mimeType, "video/") && !strings.HasPrefix(mimeType, "audio/") {
				return errors.New("公网中转仅支持图片、视频和音频")
			}
			link, err := uploadLitterbox(ctx, outbound.OutboundHTTPClient(2*time.Minute), content, mimeType)
			if err != nil {
				return err
			}
			media.URL, media.DataURL = link, ""
			media.MimeType, media.Bytes = mimeType, int64(len(content))
			media.Width, media.Height, media.DurationMs = resource.Width, resource.Height, resource.DurationMs
		}
	}
	return nil
}

func validateGiteeMode(input *canvasGenerationInput) error {
	operation := firstNonEmpty(metadataString(input.Metadata, "videoEditOperation"), metadataString(input.Metadata, "videoOperation"))
	if operation == "" && input.VideoCapability != nil {
		operation = input.VideoCapability.DefaultOperation
	}
	if input.Metadata == nil {
		input.Metadata = map[string]any{}
	}
	input.Metadata["videoEditOperation"] = operation
	count := len(input.ReferenceImages) + len(input.ReferenceVideos) + len(input.ReferenceAudios)
	switch operation {
	case "text_to_video":
		if count != 0 {
			return errors.New("文生视频不能携带参考素材，请选择对应模式")
		}
	case "image_to_video":
		if len(input.ReferenceImages) != 2 || len(input.ReferenceVideos)+len(input.ReferenceAudios) != 0 {
			return errors.New("首尾帧生成需要两张图片，不能混入视频或音频")
		}
		first, last := 0, 0
		for _, m := range input.ReferenceImages {
			switch videoImageRoleOrDefault(*input, m, "") {
			case "first_frame":
				first++
			case "last_frame":
				last++
			}
		}
		if first != 1 || last != 1 {
			return errors.New("请明确指定一张首帧和一张尾帧，不能重复或遗漏")
		}
	case "reference_to_video":
		if count == 0 {
			return errors.New("参考生成至少需要一份参考素材")
		}
		for _, m := range input.ReferenceImages {
			if m.Role == "first_frame" || m.Role == "last_frame" {
				return errors.New("参考生成不能混入首尾帧，请切换首尾帧模式")
			}
		}
	default:
		return errors.New("不支持的 Gitee H3 生成模式")
	}
	return nil
}

func uploadLitterbox(ctx context.Context, client *http.Client, content []byte, mimeType string) (string, error) {
	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	if err := writer.WriteField("reqtype", "fileupload"); err != nil {
		return "", err
	}
	if err := writer.WriteField("time", "12h"); err != nil {
		return "", err
	}
	// Generic names avoid leaking the original filename or project name.
	ext := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp", "video/mp4": ".mp4", "audio/mpeg": ".mp3", "audio/wav": ".wav", "audio/x-wav": ".wav"}[mimeType]
	if ext == "" {
		return "", errors.New("暂不支持该素材格式的公网中转")
	}
	part, err := writer.CreateFormFile("fileToUpload", "reference"+ext)
	if err != nil {
		return "", err
	}
	if _, err = part.Write(content); err != nil {
		return "", err
	}
	if err = writer.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, litterboxEndpoint, &payload)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	// The upload never carries the model provider's Authorization header.
	res, err := client.Do(req)
	if err != nil {
		return "", errors.New("Litterbox 上传失败，请稍后重试；未提交视频生成")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Litterbox 上传失败（HTTP %d）；未提交视频生成", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4097))
	if err != nil || len(raw) > 4096 {
		return "", errors.New("Litterbox 返回内容无效")
	}
	link := strings.TrimSpace(string(raw))
	parsed, err := url.Parse(link)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "litter.catbox.moe" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || path.Ext(parsed.Path) != ext {
		return "", errors.New("Litterbox 未返回有效的素材链接；未提交视频生成")
	}
	return link, nil
}
