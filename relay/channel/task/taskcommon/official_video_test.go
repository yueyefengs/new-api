package taskcommon

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestNormalizeOfficialVideoTaskRequest(t *testing.T) {
	duration := dto.IntValue(11)
	generateAudio := dto.BoolValue(true)
	watermark := dto.BoolValue(false)
	req := &OfficialVideoRequest{
		Model: "doubao-seedance-2-0",
		Content: []OfficialVideoContentItem{
			{Type: "text", Text: "prompt text"},
			{Type: "image_url", ImageURL: &OfficialVideoMediaURL{URL: "https://example.com/a.jpg"}, Role: "reference_image"},
			{Type: "video_url", VideoURL: &OfficialVideoMediaURL{URL: "https://example.com/a.mp4"}, Role: "reference_video"},
		},
		GenerateAudio: &generateAudio,
		Ratio:         "16:9",
		Duration:      &duration,
		Watermark:     &watermark,
	}

	normalized := NormalizeOfficialVideoTaskRequest(req, nil)
	require.Equal(t, "doubao-seedance-2-0", normalized.Model)
	require.Equal(t, "prompt text", normalized.Prompt)
	require.Equal(t, 11, normalized.Duration)
	require.Equal(t, "11", normalized.Seconds)
	require.Contains(t, normalized.Metadata, "content")
	require.Equal(t, "16:9", normalized.Metadata["ratio"])
	require.Equal(t, true, normalized.Metadata["generate_audio"])
	require.Equal(t, false, normalized.Metadata["watermark"])
}
