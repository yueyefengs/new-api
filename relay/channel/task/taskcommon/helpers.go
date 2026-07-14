package taskcommon

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

// UnmarshalMetadata converts a map[string]any metadata to a typed struct via JSON round-trip.
// This replaces the repeated pattern: json.Marshal(metadata) → json.Unmarshal(bytes, &target).
func UnmarshalMetadata(metadata map[string]any, target any) error {
	if metadata == nil {
		return nil
	}
	// Prevent metadata from overriding model fields to avoid billing bypass.
	delete(metadata, "model")
	metaBytes, err := common.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata failed: %w", err)
	}
	if err := common.Unmarshal(metaBytes, target); err != nil {
		return fmt.Errorf("unmarshal metadata failed: %w", err)
	}
	return nil
}

// DefaultString returns val if non-empty, otherwise fallback.
func DefaultString(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}

// DefaultInt returns val if non-zero, otherwise fallback.
func DefaultInt(val, fallback int) int {
	if val == 0 {
		return fallback
	}
	return val
}

// EncodeLocalTaskID encodes an upstream operation name to a URL-safe base64 string.
// Used by Gemini/Vertex to store upstream names as task IDs.
func EncodeLocalTaskID(name string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(name))
}

// DecodeLocalTaskID decodes a base64-encoded upstream operation name.
func DecodeLocalTaskID(id string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// BuildProxyURL constructs the video proxy URL using the public task ID.
// e.g., "https://your-server.com/v1/videos/task_xxxx/content"
func BuildProxyURL(taskID string) string {
	return fmt.Sprintf("%s/v1/videos/%s/content", system_setting.ServerAddress, taskID)
}

// Status-to-progress mapping constants for polling updates.
const (
	ProgressSubmitted  = "10%"
	ProgressQueued     = "20%"
	ProgressInProgress = "30%"
	ProgressComplete   = "100%"
)

// ---------------------------------------------------------------------------
// BaseBilling — embeddable no-op implementations for TaskAdaptor billing methods.
// Adaptors that do not need custom billing can embed this struct directly.
// ---------------------------------------------------------------------------

type BaseBilling struct{}

// EstimateBilling returns nil (no extra ratios; use base model price).
func (BaseBilling) EstimateBilling(_ *gin.Context, _ *relaycommon.RelayInfo) map[string]float64 {
	return nil
}

// AdjustBillingOnSubmit returns nil (no submit-time adjustment).
func (BaseBilling) AdjustBillingOnSubmit(_ *relaycommon.RelayInfo, _ []byte) map[string]float64 {
	return nil
}

// AdjustBillingOnComplete returns 0 (keep pre-charged amount).
func (BaseBilling) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return 0
}

// ResolveQualitySizeRatio maps a resolution string (e.g. "720p", "1080p", "4k")
// to a billing size multiplier relative to 480p baseline.
// 480p=1.0, 720p=2.0, 1080p=5.0, 4k=10.0
func ResolveQualitySizeRatio(resolution string) float64 {
	r := strings.ToLower(strings.TrimSpace(resolution))
	switch {
	case strings.Contains(r, "4k") || strings.Contains(r, "8k"):
		return 10.0
	case strings.Contains(r, "1080") || strings.Contains(r, "2k"):
		return 5.0
	case strings.Contains(r, "720"):
		return 2.0
	default:
		return 1.0
	}
}

// ResolveDuration extracts duration in seconds from a TaskSubmitReq.
// Falls back to 5 seconds if not specified.
func ResolveDuration(req relaycommon.TaskSubmitReq) int {
	if req.Duration > 0 {
		return req.Duration
	}
	seconds, _ := strconv.Atoi(strings.TrimSpace(req.Seconds))
	if seconds > 0 {
		return seconds
	}
	return 5
}

// DefaultEstimateBilling returns OtherRatios with seconds and size based on the request.
// Adaptors can call this and append channel-specific ratios (e.g. video_input discount).
func DefaultEstimateBilling(req relaycommon.TaskSubmitReq) map[string]float64 {
	ratios := map[string]float64{"seconds": float64(ResolveDuration(req))}

	resolution := ""
	if req.Metadata != nil {
		if raw, ok := req.Metadata["resolution"]; ok {
			resolution = fmt.Sprintf("%v", raw)
		}
	}
	sizeRatio := ResolveQualitySizeRatio(resolution)
	if sizeRatio > 0 && sizeRatio != 1.0 {
		ratios["size"] = sizeRatio
	}
	return ratios
}
