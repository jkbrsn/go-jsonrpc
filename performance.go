package jsonrpc

import (
	"sync"

	"github.com/bytedance/sonic"
)

// PerformanceProfile defines a set of sonic config options optimized for different use cases. Each
// profile represents a different trade-off between performance, safety, and compatibility.
type PerformanceProfile int

const (
	// ProfileDefault uses sonic's default configuration, providing a balance of efficiency
	// and safety. Provides excellent performance without sacrificing robustness.
	//
	// Characteristics:
	//   - No HTML escaping (JSON-RPC doesn't need it)
	//   - No map key sorting (determinism not required)
	//   - Full validation enabled
	//   - String copying enabled (safer for concurrent use)
	ProfileDefault PerformanceProfile = iota

	// ProfileCompatible mimics encoding/json behavior for maximum compatibility.
	// Use this when migrating from encoding/json and you need identical output formatting.
	//
	// Characteristics:
	//   - HTML escaping enabled (slower)
	//   - Map key sorting enabled (slower, but deterministic)
	//   - Compact marshaling
	//   - String copying enabled
	//   - Full validation enabled
	//
	// Note: This is SLOWER than ProfileDefault due to sorting and escaping
	ProfileCompatible

	// ProfileBalanced applies safe performance optimizations without compromising robustness.
	// This profile disables unnecessary features (HTML escaping, key sorting) while keeping
	// all safety validations enabled.
	//
	// Characteristics:
	//   - No HTML escaping (JSON-RPC safe)
	//   - No map key sorting (faster)
	//   - Compact marshaling
	//   - No null slices/maps (cleaner JSON)
	//   - Full validation enabled
	//   - String copying enabled
	ProfileBalanced

	// ProfileFast uses sonic's official ConfigFastest settings, prioritizing speed while
	// maintaining reasonable safety. Skips validation of already-validated JSON.
	//
	// Characteristics:
	//   - Skips validation of json.Marshaler output
	//   - Skips validation when skipping unknown fields
	//   - All other safety features enabled
	//
	// Based on: sonic.ConfigFastest (official recommendation for a fast profile)
	ProfileFast

	// ProfileAggressive maximizes performance by disabling most validation and safety features.
	// Only use this when you have complete control over input sources and have profiled to
	// confirm JSON encoding is a bottleneck.
	//
	// Characteristics:
	//   - Zero-copy string conversion (unsafe with reused buffers)
	//   - No UTF-8 validation (panics on invalid UTF-8)
	//   - No JSON validation for marshalers
	//   - No HTML escaping
	//   - No map key sorting
	//   - Compact marshaling
	//   - No null for empty slices/maps
	//
	// WARNING: Can panic or produce invalid JSON with malformed input
	ProfileAggressive
)

var (
	// currentProfile tracks the active performance profile
	currentProfile = ProfileDefault

	// sonicAPI is the configured sonic API instance used for all JSON operations
	sonicAPI sonic.API = sonic.ConfigDefault

	// profileMutex protects profile changes
	profileMutex sync.RWMutex

	// Pre-configured sonic API instances for each profile
	profileConfigs = map[PerformanceProfile]sonic.API{
		ProfileDefault: sonic.ConfigDefault,

		ProfileCompatible: sonic.Config{
			EscapeHTML:       true, // encoding/json compatibility
			SortMapKeys:      true, // encoding/json compatibility
			CompactMarshaler: true,
			CopyString:       true,
			ValidateString:   true,
		}.Froze(),

		ProfileBalanced: sonic.Config{
			EscapeHTML:       false, // JSON-RPC doesn't contain HTML
			SortMapKeys:      false, // Determinism not required
			CompactMarshaler: true,  // No whitespace
			NoNullSliceOrMap: true,  // Cleaner JSON output
			CopyString:       true,  // Safety over speed
			ValidateString:   true,  // Validate UTF-8
		}.Froze(),

		ProfileFast: sonic.ConfigFastest,

		ProfileAggressive: sonic.Config{
			CopyString:              false, // Zero-copy (unsafe with buffer reuse)
			NoNullSliceOrMap:        true,  // Cleaner JSON
			NoValidateJSONMarshaler: true,  // Skip validation
			NoValidateJSONSkip:      true,  // Skip validation
			EscapeHTML:              false, // No escaping
			SortMapKeys:             false, // No sorting
			CompactMarshaler:        true,  // No whitespace
			ValidateString:          false, // No UTF-8 validation
		}.Froze(),
	}
)

// SetPerformanceProfile configures the JSON encoding/decoding behavior for all operations.
// This function is thread-safe and affects all subsequent JSON operations in the package.
//
// The profile determines the trade-off between performance, safety, and compatibility:
//
//   - ProfileDefault: Recommended for most users (efficient + safe)
//   - ProfileCompatible: Use when migrating from encoding/json
//   - ProfileBalanced: Production apps wanting safe optimizations
//   - ProfileFast: Internal services with controlled data
//   - ProfileAggressive: Maximum speed, use with caution
//
// Example usage:
//
//	// Use the balanced profile
//	jsonrpc.SetPerformanceProfile(jsonrpc.ProfileBalanced)
func SetPerformanceProfile(profile PerformanceProfile) {
	profileMutex.Lock()
	defer profileMutex.Unlock()

	if cfg, ok := profileConfigs[profile]; ok {
		sonicAPI = cfg
		currentProfile = profile
	}
}

// GetPerformanceProfile returns the currently active performance profile.
// This function is thread-safe.
func GetPerformanceProfile() PerformanceProfile {
	profileMutex.RLock()
	defer profileMutex.RUnlock()
	return currentProfile
}

// getSonicAPI returns the current sonic API instance for JSON operations.
// This is an internal helper used by marshal/unmarshal functions.
func getSonicAPI() sonic.API {
	profileMutex.RLock()
	defer profileMutex.RUnlock()
	return sonicAPI
}
