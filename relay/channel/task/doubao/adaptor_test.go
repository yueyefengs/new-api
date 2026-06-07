package doubao

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResolveModelAlias(t *testing.T) {
	require.Equal(t, "doubao-seedance-2-0-260128", ResolveModelAlias("doubao-seedance-2-0"))
	require.Equal(t, "doubao-seedance-2-0-260128", ResolveModelAlias("doubao-seedance-2-0-260128"))
}

func TestGetVideoInputRatioSupportsAlias(t *testing.T) {
	aliasRatio, ok := GetVideoInputRatio("doubao-seedance-2-0")
	require.True(t, ok)
	versionRatio, ok := GetVideoInputRatio("doubao-seedance-2-0-260128")
	require.True(t, ok)
	require.Equal(t, versionRatio, aliasRatio)
}

func TestConvertToRequestPayloadResolvesAlias(t *testing.T) {
	adaptor := &TaskAdaptor{}
	payload, err := adaptor.convertToRequestPayload(&relaycommon.TaskSubmitReq{
		Model:   "doubao-seedance-2-0",
		Prompt:  "test prompt",
		Seconds: "11",
	})
	require.NoError(t, err)
	require.Equal(t, "doubao-seedance-2-0-260128", payload.Model)
	require.NotNil(t, payload.Duration)
	require.Equal(t, 11, int(*payload.Duration))
	require.Len(t, payload.Content, 1)
	require.Equal(t, "text", payload.Content[0].Type)
	require.Equal(t, "test prompt", payload.Content[0].Text)
}

func TestConvertToRequestPayloadSupportsOfficialNormalizedContent(t *testing.T) {
	duration := dto.IntValue(11)
	request := &taskcommon.OfficialVideoRequest{
		Model: "doubao-seedance-2-0",
		Content: []taskcommon.OfficialVideoContentItem{
			{Type: "text", Text: "prompt text"},
			{Type: "image_url", ImageURL: &taskcommon.OfficialVideoMediaURL{URL: "https://example.com/a.jpg"}, Role: "reference_image"},
			{Type: "audio_url", AudioURL: &taskcommon.OfficialVideoMediaURL{URL: "https://example.com/a.mp3"}, Role: "reference_audio"},
		},
		Duration: &duration,
	}
	normalized := taskcommon.NormalizeOfficialVideoTaskRequest(request, modelAliasMap)
	adaptor := &TaskAdaptor{}
	payload, err := adaptor.convertToRequestPayload(&normalized)
	require.NoError(t, err)
	require.Equal(t, "doubao-seedance-2-0-260128", payload.Model)
	require.Len(t, payload.Content, 3)
	require.Equal(t, "image_url", payload.Content[0].Type)
	require.Equal(t, "audio_url", payload.Content[1].Type)
	require.Equal(t, "text", payload.Content[2].Type)
}

func TestValidateRequestAndSetActionSupportsOfficialVideoContentOnlyRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"doubao-seedance-2-0",
		"content":[{"type":"text","text":"一只机械鲸鱼跃出海面，电影感镜头，真实光影。"}],
		"generate_audio":false,
		"ratio":"16:9",
		"duration":5,
		"watermark":false
	}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{}
	err := adaptor.ValidateRequestAndSetAction(c, info)
	require.Nil(t, err)

	req, getErr := relaycommon.GetTaskRequest(c)
	require.NoError(t, getErr)
	require.Equal(t, "doubao-seedance-2-0-260128", req.Model)
	require.Equal(t, "一只机械鲸鱼跃出海面，电影感镜头，真实光影。", req.Prompt)
	require.Equal(t, 5, req.Duration)
	require.Equal(t, "5", req.Seconds)
	require.Equal(t, "16:9", req.Metadata["ratio"])
	require.Equal(t, false, req.Metadata["generate_audio"])
	require.Equal(t, false, req.Metadata["watermark"])
}
