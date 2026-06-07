package chengmeng

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

const (
	defaultOrientation = "landscape"
	defaultSize        = "large"
	maxPromptLength    = 1500
	maxImageCount      = 4
	maxVideoCount      = 3
	minDuration        = 5
	maxDuration        = 15
)

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	apiKey      string
	baseURL     string
}

type requestPayload struct {
	ModelID  string        `json:"model_id"`
	GroupID  string        `json:"group_id"`
	Prompt   string        `json:"prompt"`
	Duration int           `json:"duration"`
	Values   requestValues `json:"values"`
	Images   []string      `json:"images,omitempty"`
}

type requestValues struct {
	Orientation string   `json:"orientation"`
	Size        string   `json:"size,omitempty"`
	Videos      []string `json:"videos,omitempty"`
}

type requestMetadata struct {
	Orientation string   `json:"orientation,omitempty"`
	Size        string   `json:"size,omitempty"`
	Videos      []string `json:"videos,omitempty"`
	ModelID     string   `json:"model_id,omitempty"`
	GroupID     string   `json:"group_id,omitempty"`
}

type channelOtherConfig struct {
	ModelMapping map[string]ModelConfig `json:"model_mapping,omitempty"`
}

type createResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		TaskNo        string  `json:"task_no"`
		Status        string  `json:"status"`
		EstimatedCost float64 `json:"estimated_cost"`
		FrozenAmount  float64 `json:"frozen_amount"`
	} `json:"data"`
}

type fetchResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		ID          int     `json:"id"`
		TaskNo      string  `json:"task_no"`
		Status      string  `json:"status"`
		Prompt      string  `json:"prompt"`
		ResultURL   string  `json:"result_url"`
		DownloadURL string  `json:"download_url"`
		ActualCost  float64 `json:"actual_cost"`
		InputParams struct {
			Orientation string `json:"orientation"`
			Size        string `json:"size"`
			Watermark   bool   `json:"watermark"`
		} `json:"input_params"`
		StartedAt  string `json:"started_at"`
		FinishedAt string `json:"finished_at"`
	} `json:"data"`
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.apiKey = info.ApiKey
	a.baseURL = info.ChannelBaseUrl
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if info.TaskRelayInfo == nil {
		info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
	}
	officialReq, err := taskcommon.ParseOfficialVideoRequest(c)
	if err == nil && taskcommon.IsOfficialVideoRequest(officialReq) {
		normalized, taskErr := normalizeOfficialRequestForChengmeng(officialReq)
		if taskErr != nil {
			return taskErr
		}
		if strings.TrimSpace(normalized.Prompt) == "" {
			return service.TaskErrorWrapperLocal(fmt.Errorf("prompt is required"), "invalid_request", http.StatusBadRequest)
		}
		if strings.TrimSpace(normalized.Model) == "" {
			return service.TaskErrorWrapperLocal(fmt.Errorf("model field is required"), "missing_model", http.StatusBadRequest)
		}
		info.Action = constant.TaskActionGenerate
		c.Set("task_request", normalized)
	} else {
		if err := relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate); err != nil {
			return err
		}
	}

	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_task_request_failed", http.StatusBadRequest)
	}

	if strings.TrimSpace(taskcommon.DefaultString(info.UpstreamModelName, req.Model)) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("model field is required"), "missing_model", http.StatusBadRequest)
	}

	metadata, err := parseRequestMetadata(req.Metadata)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_metadata", http.StatusBadRequest)
	}

	duration, err := resolveDuration(req)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_duration", http.StatusBadRequest)
	}
	if duration < minDuration || duration > maxDuration {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("duration must be between %d and %d seconds", minDuration, maxDuration),
			"invalid_duration",
			http.StatusBadRequest,
		)
	}

	if len(req.Images) > maxImageCount {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("at most %d images are supported", maxImageCount),
			"too_many_images",
			http.StatusBadRequest,
		)
	}
	if len(metadata.Videos) > maxVideoCount {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("at most %d videos are supported", maxVideoCount),
			"too_many_videos",
			http.StatusBadRequest,
		)
	}

	prompt := buildPromptWithReferences(req.Prompt, req.Images, metadata.Videos)
	if utf8.RuneCountInString(prompt) > maxPromptLength {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("prompt exceeds %d characters after adding media references", maxPromptLength),
			"prompt_too_long",
			http.StatusBadRequest,
		)
	}

	return nil
}

func normalizeOfficialRequestForChengmeng(req *taskcommon.OfficialVideoRequest) (relaycommon.TaskSubmitReq, *dto.TaskError) {
	normalized := taskcommon.NormalizeOfficialVideoTaskRequest(req, ModelAliasMap)
	summary := taskcommon.SummarizeOfficialVideoContent(req)
	if len(summary.Audios) > 0 {
		return relaycommon.TaskSubmitReq{}, service.TaskErrorWrapperLocal(
			fmt.Errorf("audio_url is not supported by Chengmeng"),
			"unsupported_audio_reference",
			http.StatusBadRequest,
		)
	}
	if req.GenerateAudio != nil && bool(*req.GenerateAudio) {
		return relaycommon.TaskSubmitReq{}, service.TaskErrorWrapperLocal(
			fmt.Errorf("generate_audio is not supported by Chengmeng"),
			"unsupported_generate_audio",
			http.StatusBadRequest,
		)
	}
	normalized.Images = summary.Images
	if normalized.Metadata == nil {
		normalized.Metadata = map[string]interface{}{}
	}
	delete(normalized.Metadata, "content")
	if len(summary.Videos) > 0 {
		normalized.Metadata["videos"] = summary.Videos
	}
	if orientation, ok := mapRatioToOrientation(strings.TrimSpace(req.Ratio)); ok {
		normalized.Metadata["orientation"] = orientation
	}
	if _, ok := normalized.Metadata["size"]; !ok {
		normalized.Metadata["size"] = defaultSize
	}
	return normalized, nil
}

func mapRatioToOrientation(ratio string) (string, bool) {
	switch ratio {
	case "16:9":
		return "landscape", true
	case "9:16":
		return "portrait", true
	default:
		return "", false
	}
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, _ *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	duration, err := resolveDuration(req)
	if err != nil {
		return nil
	}
	return map[string]float64{"seconds": float64(duration)}
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/api/tasks", a.baseURL), nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}

	metadata, err := parseRequestMetadata(req.Metadata)
	if err != nil {
		return nil, err
	}

	modelName := taskcommon.DefaultString(info.UpstreamModelName, req.Model)
	if modelName == "" {
		modelName = info.OriginModelName
	}
	if info.UpstreamModelName == "" {
		info.UpstreamModelName = modelName
	}

	modelMapping, err := loadModelMapping(info.ChannelId)
	if err != nil {
		return nil, err
	}
	modelConfig, err := resolveModelConfig(modelName, metadata, modelMapping)
	if err != nil {
		return nil, err
	}

	duration, err := resolveDuration(req)
	if err != nil {
		return nil, err
	}

	payload := requestPayload{
		ModelID:  modelConfig.ModelID,
		GroupID:  modelConfig.GroupID,
		Prompt:   buildPromptWithReferences(req.Prompt, req.Images, metadata.Videos),
		Duration: duration,
		Values: requestValues{
			Orientation: taskcommon.DefaultString(metadata.Orientation, defaultOrientation),
			Size:        resolveSize(req, metadata),
			Videos:      metadata.Videos,
		},
		Images: req.Images,
	}

	body, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	common.SysLog(fmt.Sprintf("[chengmeng] upstream request body: %s", string(body)))
	return bytes.NewReader(body), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	var cmResp createResponse
	if err = common.Unmarshal(responseBody, &cmResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if cmResp.Code != 0 {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("chengmeng api error: %s", cmResp.Message), strconv.Itoa(cmResp.Code), http.StatusBadRequest)
		return
	}
	if cmResp.Data.TaskNo == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_no is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName

	c.JSON(http.StatusOK, ov)
	return cmResp.Data.TaskNo, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/api/tasks/%s", baseURL, taskID)
	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var cmResp fetchResponse
	if err := common.Unmarshal(respBody, &cmResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}
	if cmResp.Code != 0 {
		return nil, fmt.Errorf("chengmeng api error: %s", cmResp.Message)
	}

	taskInfo := &relaycommon.TaskInfo{Code: cmResp.Code}
	switch strings.ToLower(cmResp.Data.Status) {
	case "pending":
		taskInfo.Status = string(model.TaskStatusQueued)
		taskInfo.Progress = "10%"
	case "running":
		taskInfo.Status = string(model.TaskStatusInProgress)
		taskInfo.Progress = "50%"
	case "completed", "success":
		taskInfo.Status = string(model.TaskStatusSuccess)
		taskInfo.Progress = "100%"
		taskInfo.Url = firstNonEmpty(cmResp.Data.ResultURL, cmResp.Data.DownloadURL)
	case "failed", "error", "cancelled":
		taskInfo.Status = string(model.TaskStatusFailure)
		taskInfo.Progress = "100%"
		taskInfo.Reason = firstNonEmpty(cmResp.Message, "task failed")
	default:
		taskInfo.Status = string(model.TaskStatusInProgress)
		taskInfo.Progress = taskcommon.ProgressInProgress
	}

	return taskInfo, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	video := originTask.ToOpenAIVideo()
	video.TaskID = originTask.TaskID
	if strings.TrimSpace(originTask.GetResultURL()) != "" {
		video.SetMetadata("url", originTask.GetResultURL())
	}
	if originTask.Status == model.TaskStatusFailure && strings.TrimSpace(originTask.FailReason) != "" {
		video.Error = &dto.OpenAIVideoError{
			Message: originTask.FailReason,
			Code:    "task_failed",
		}
	}
	return common.Marshal(video)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func parseRequestMetadata(metadata map[string]any) (requestMetadata, error) {
	var result requestMetadata
	if err := taskcommon.UnmarshalMetadata(metadata, &result); err != nil {
		return result, err
	}
	return result, nil
}

func resolveDuration(req relaycommon.TaskSubmitReq) (int, error) {
	if req.Duration > 0 {
		return req.Duration, nil
	}
	if strings.TrimSpace(req.Seconds) == "" {
		return 0, fmt.Errorf("seconds is required")
	}
	duration, err := strconv.Atoi(strings.TrimSpace(req.Seconds))
	if err != nil {
		return 0, fmt.Errorf("invalid seconds: %w", err)
	}
	return duration, nil
}

func resolveSize(req relaycommon.TaskSubmitReq, metadata requestMetadata) string {
	if metadata.Size != "" {
		return metadata.Size
	}
	if req.Size != "" {
		return req.Size
	}
	return defaultSize
}

func buildPromptWithReferences(prompt string, images []string, videos []string) string {
	trimmedPrompt := strings.TrimSpace(prompt)
	references := make([]string, 0, len(images)+len(videos))
	for i := range images {
		token := fmt.Sprintf("@图片%d", i+1)
		if !strings.Contains(trimmedPrompt, token) {
			references = append(references, token)
		}
	}
	for i := range videos {
		token := fmt.Sprintf("@素材%d", i+1)
		if !strings.Contains(trimmedPrompt, token) {
			references = append(references, token)
		}
	}
	if len(references) == 0 {
		return trimmedPrompt
	}
	if trimmedPrompt == "" {
		return strings.Join(references, " ")
	}
	return trimmedPrompt + " " + strings.Join(references, " ")
}

func resolveModelConfig(modelName string, metadata requestMetadata, mapping map[string]ModelConfig) (ModelConfig, error) {
	config, ok := mapping[modelName]
	if !ok {
		return ModelConfig{}, fmt.Errorf("unsupported model: %s", modelName)
	}
	if metadata.ModelID != "" {
		config.ModelID = metadata.ModelID
	}
	if metadata.GroupID != "" {
		config.GroupID = metadata.GroupID
	}
	if config.ModelID == "" || config.GroupID == "" {
		return ModelConfig{}, fmt.Errorf("model_id and group_id are required for model %s", modelName)
	}
	return config, nil
}

func loadModelMapping(channelID int) (map[string]ModelConfig, error) {
	mapping := cloneModelMapping(DefaultModelMapping)
	if channelID <= 0 {
		return mapping, nil
	}

	ch, err := model.CacheGetChannel(channelID)
	if err != nil || ch == nil || strings.TrimSpace(ch.Other) == "" {
		return mapping, nil
	}

	var config channelOtherConfig
	if err := common.UnmarshalJsonStr(ch.Other, &config); err != nil {
		return nil, fmt.Errorf("invalid channel other config: %w", err)
	}
	for name, modelConfig := range config.ModelMapping {
		mapping[name] = modelConfig
	}
	return mapping, nil
}

func cloneModelMapping(src map[string]ModelConfig) map[string]ModelConfig {
	result := make(map[string]ModelConfig, len(src))
	for key, value := range src {
		result[key] = value
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
