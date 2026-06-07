package chengmeng

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestBuildPromptWithReferences(t *testing.T) {
	prompt := buildPromptWithReferences("一只猫在奔跑 @图片1", []string{"a", "b"}, []string{"v1"})
	require.Equal(t, "一只猫在奔跑 @图片1 @图片2 @素材1", prompt)
}

func TestResolveModelConfigMetadataOverride(t *testing.T) {
	config, err := resolveModelConfig("doubao-seedance-2-0", requestMetadata{
		ModelID: "99",
		GroupID: "77",
	}, DefaultModelMapping)
	require.NoError(t, err)
	require.Equal(t, ModelConfig{ModelID: "99", GroupID: "77"}, config)
}

func TestParseTaskResult(t *testing.T) {
	adaptor := &TaskAdaptor{}
	result, err := adaptor.ParseTaskResult([]byte(`{"code":0,"message":"ok","data":{"status":"completed","result_url":"https://example.com/video.mp4"}}`))
	require.NoError(t, err)
	require.Equal(t, string(model.TaskStatusSuccess), result.Status)
	require.Equal(t, "100%", result.Progress)
	require.Equal(t, "https://example.com/video.mp4", result.Url)
}

func TestResolveDuration(t *testing.T) {
	duration, err := resolveDuration(relaycommon.TaskSubmitReq{Seconds: "10"})
	require.NoError(t, err)
	require.Equal(t, 10, duration)

	duration, err = resolveDuration(relaycommon.TaskSubmitReq{Duration: 12})
	require.NoError(t, err)
	require.Equal(t, 12, duration)
}

func TestNormalizeOfficialRequestForChengmeng(t *testing.T) {
	duration := dto.IntValue(11)
	req := &taskcommon.OfficialVideoRequest{
		Model: "doubao-seedance-2-0",
		Content: []taskcommon.OfficialVideoContentItem{
			{Type: "text", Text: "prompt text"},
			{Type: "image_url", ImageURL: &taskcommon.OfficialVideoMediaURL{URL: "https://example.com/a.jpg"}, Role: "reference_image"},
			{Type: "video_url", VideoURL: &taskcommon.OfficialVideoMediaURL{URL: "https://example.com/a.mp4"}, Role: "reference_video"},
		},
		Ratio:    "16:9",
		Duration: &duration,
	}

	normalized, taskErr := normalizeOfficialRequestForChengmeng(req)
	require.Nil(t, taskErr)
	require.Equal(t, "seedance-2-0", normalized.Model)
	require.Equal(t, []string{"https://example.com/a.jpg"}, normalized.Images)
	require.Equal(t, []string{"https://example.com/a.mp4"}, normalized.Metadata["videos"])
	require.Equal(t, "landscape", normalized.Metadata["orientation"])
	require.Equal(t, defaultSize, normalized.Metadata["size"])
}

func TestNormalizeOfficialRequestForChengmengRejectsAudio(t *testing.T) {
	duration := dto.IntValue(11)
	req := &taskcommon.OfficialVideoRequest{
		Model: "doubao-seedance-2-0",
		Content: []taskcommon.OfficialVideoContentItem{
			{Type: "text", Text: "prompt text"},
			{Type: "audio_url", AudioURL: &taskcommon.OfficialVideoMediaURL{URL: "https://example.com/a.mp3"}, Role: "reference_audio"},
		},
		Duration: &duration,
	}

	_, taskErr := normalizeOfficialRequestForChengmeng(req)
	require.NotNil(t, taskErr)
	require.Equal(t, "unsupported_audio_reference", taskErr.Code)
}
