package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExpandModelMatchingCandidates(t *testing.T) {
	candidates := ExpandModelMatchingCandidates("doubao-seedance-2-0")
	require.Contains(t, candidates, "doubao-seedance-2-0")
	require.Contains(t, candidates, "seedance-2-0")
}

func TestIsModelOrEquivalentEnabled(t *testing.T) {
	require.True(t, IsModelOrEquivalentEnabled(map[string]bool{"seedance-2-0": true}, "doubao-seedance-2-0"))
	require.True(t, IsModelOrEquivalentEnabled(map[string]bool{"doubao-seedance-2-0": true}, "seedance-2-0"))
	require.False(t, IsModelOrEquivalentEnabled(map[string]bool{"other-model": true}, "seedance-2-0"))
}

func TestExpandModelMatchingCandidatesPreferredPublicNameFirst(t *testing.T) {
	candidates := ExpandModelMatchingCandidates("doubao-seedance-2-0")
	require.Equal(t, "doubao-seedance-2-0", candidates[0])
}
