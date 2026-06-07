package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIModelsIncludeTaskOnlyVideoModels(t *testing.T) {
	require.Contains(t, openAIModelsMap, "doubao-seedance-2-0")
	model, ok := openAIModelsMap["doubao-seedance-2-0"]
	require.True(t, ok)
	require.Equal(t, "chengmeng-video", model.OwnedBy)
	require.Contains(t, channelId2Models, 158)
	require.Contains(t, channelId2Models[158], "doubao-seedance-2-0")
}
