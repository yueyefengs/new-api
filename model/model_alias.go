package model

import "github.com/QuantumNous/new-api/setting/ratio_setting"

var equivalentModelAliases = map[string][]string{
	"seedance-2-0":        {"doubao-seedance-2-0"},
	"doubao-seedance-2-0": {"seedance-2-0"},
}

func ExpandModelMatchingCandidates(modelName string) []string {
	if modelName == "" {
		return nil
	}

	seen := map[string]struct{}{}
	result := make([]string, 0, 4)
	appendName := func(name string) {
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}

	appendName(modelName)
	appendName(ratio_setting.FormatMatchingModelName(modelName))
	for _, alias := range equivalentModelAliases[modelName] {
		appendName(alias)
		appendName(ratio_setting.FormatMatchingModelName(alias))
	}
	return result
}

func IsModelOrEquivalentEnabled(modelLimit map[string]bool, modelName string) bool {
	for _, candidate := range ExpandModelMatchingCandidates(modelName) {
		if modelLimit[candidate] {
			return true
		}
	}
	return false
}
