package taskcommon

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

type OfficialVideoMediaURL struct {
	URL string `json:"url,omitempty"`
}

type OfficialVideoContentItem struct {
	Type     string                 `json:"type,omitempty"`
	Text     string                 `json:"text,omitempty"`
	ImageURL *OfficialVideoMediaURL `json:"image_url,omitempty"`
	VideoURL *OfficialVideoMediaURL `json:"video_url,omitempty"`
	AudioURL *OfficialVideoMediaURL `json:"audio_url,omitempty"`
	Role     string                 `json:"role,omitempty"`
}

type OfficialVideoRequest struct {
	Model         string                     `json:"model,omitempty"`
	Prompt        string                     `json:"prompt,omitempty"`
	Content       []OfficialVideoContentItem `json:"content,omitempty"`
	GenerateAudio *dto.BoolValue             `json:"generate_audio,omitempty"`
	Ratio         string                     `json:"ratio,omitempty"`
	Duration      *dto.IntValue              `json:"duration,omitempty"`
	Watermark     *dto.BoolValue             `json:"watermark,omitempty"`
}

type OfficialVideoSummary struct {
	Prompt         string
	NonTextContent []OfficialVideoContentItem
	Images         []string
	Videos         []string
	Audios         []string
}

func ParseOfficialVideoRequest(c *gin.Context) (*OfficialVideoRequest, error) {
	var req OfficialVideoRequest
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func IsOfficialVideoRequest(req *OfficialVideoRequest) bool {
	return req != nil && len(req.Content) > 0
}

func SummarizeOfficialVideoContent(req *OfficialVideoRequest) OfficialVideoSummary {
	summary := OfficialVideoSummary{
		Prompt: strings.TrimSpace(req.Prompt),
	}
	textParts := make([]string, 0, len(req.Content))
	for _, item := range req.Content {
		switch item.Type {
		case "text":
			if text := strings.TrimSpace(item.Text); text != "" {
				textParts = append(textParts, text)
			}
		case "image_url":
			summary.NonTextContent = append(summary.NonTextContent, item)
			if item.ImageURL != nil && strings.TrimSpace(item.ImageURL.URL) != "" {
				summary.Images = append(summary.Images, strings.TrimSpace(item.ImageURL.URL))
			}
		case "video_url":
			summary.NonTextContent = append(summary.NonTextContent, item)
			if item.VideoURL != nil && strings.TrimSpace(item.VideoURL.URL) != "" {
				summary.Videos = append(summary.Videos, strings.TrimSpace(item.VideoURL.URL))
			}
		case "audio_url":
			summary.NonTextContent = append(summary.NonTextContent, item)
			if item.AudioURL != nil && strings.TrimSpace(item.AudioURL.URL) != "" {
				summary.Audios = append(summary.Audios, strings.TrimSpace(item.AudioURL.URL))
			}
		default:
			summary.NonTextContent = append(summary.NonTextContent, item)
		}
	}
	if summary.Prompt == "" {
		summary.Prompt = strings.Join(textParts, "\n")
	}
	return summary
}

func NormalizeOfficialVideoTaskRequest(req *OfficialVideoRequest, modelAliases map[string]string) relaycommon.TaskSubmitReq {
	modelName := strings.TrimSpace(req.Model)
	if mapped, ok := modelAliases[modelName]; ok {
		modelName = mapped
	}
	summary := SummarizeOfficialVideoContent(req)
	metadata := map[string]interface{}{}
	if len(summary.NonTextContent) > 0 {
		metadata["content"] = summary.NonTextContent
	}
	if req.GenerateAudio != nil {
		metadata["generate_audio"] = bool(*req.GenerateAudio)
	}
	if ratio := strings.TrimSpace(req.Ratio); ratio != "" {
		metadata["ratio"] = ratio
	}
	if req.Watermark != nil {
		metadata["watermark"] = bool(*req.Watermark)
	}
	normalized := relaycommon.TaskSubmitReq{
		Model:    modelName,
		Prompt:   summary.Prompt,
		Metadata: metadata,
	}
	if req.Duration != nil {
		normalized.Duration = int(*req.Duration)
		normalized.Seconds = strconv.Itoa(int(*req.Duration))
	}
	return normalized
}
