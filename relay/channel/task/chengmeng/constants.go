package chengmeng

var ModelList = []string{
	"doubao-seedance-2-0",
}

const ChannelName = "chengmeng-video"

type ModelConfig struct {
	ModelID string `json:"model_id"`
	GroupID string `json:"group_id"`
}

var DefaultModelMapping = map[string]ModelConfig{
	"doubao-seedance-2-0": {
		ModelID: "31",
		GroupID: "15",
	},
	"seedance-2-0": {
		ModelID: "31",
		GroupID: "15",
	},
}

var ModelAliasMap = map[string]string{
	"doubao-seedance-2-0": "seedance-2-0",
}
