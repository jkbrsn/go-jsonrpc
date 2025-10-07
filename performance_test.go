package jsonrpc

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetPerformanceProfile(t *testing.T) {
	// Save original profile to restore after tests
	originalProfile := GetPerformanceProfile()
	defer SetPerformanceProfile(originalProfile)

	profiles := []PerformanceProfile{
		ProfileDefault,
		ProfileCompatible,
		ProfileBalanced,
		ProfileFast,
		ProfileAggressive,
	}

	for _, profile := range profiles {
		t.Run(profile.String(), func(t *testing.T) {
			SetPerformanceProfile(profile)
			assert.Equal(t, profile, GetPerformanceProfile())
		})
	}
}

func TestGetPerformanceProfile(t *testing.T) {
	// Default should be ProfileDefault
	assert.Equal(t, ProfileDefault, GetPerformanceProfile())
}

func TestProfileDefault(t *testing.T) {
	SetPerformanceProfile(ProfileDefault)
	defer SetPerformanceProfile(ProfileDefault)

	// Test that basic marshaling works
	req := NewRequest("test_method", []any{1, 2, 3})
	data, err := req.MarshalJSON()
	require.NoError(t, err)
	assert.Contains(t, string(data), "test_method")
}

func TestProfileCompatible(t *testing.T) {
	SetPerformanceProfile(ProfileCompatible)
	defer SetPerformanceProfile(ProfileDefault)

	// ProfileCompatible should escape HTML and sort map keys
	req := NewRequest("test", map[string]any{
		"zebra":  "last",
		"apple":  "first",
		"middle": "mid",
	})
	data, err := req.MarshalJSON()
	require.NoError(t, err)

	// Check that keys are sorted (apple, middle, zebra)
	str := string(data)
	appleIdx := strings.Index(str, `"apple"`)
	middleIdx := strings.Index(str, `"middle"`)
	zebraIdx := strings.Index(str, `"zebra"`)

	assert.True(t, appleIdx < middleIdx, "apple should come before middle")
	assert.True(t, middleIdx < zebraIdx, "middle should come before zebra")
}

func TestProfileBalanced(t *testing.T) {
	SetPerformanceProfile(ProfileBalanced)
	defer SetPerformanceProfile(ProfileDefault)

	// ProfileBalanced should handle empty slices as [] not null
	type TestData struct {
		EmptySlice []string `json:"empty_slice"`
		Value      string   `json:"value"`
	}

	resp, err := NewResponse(1, TestData{
		EmptySlice: []string{},
		Value:      "test",
	})
	require.NoError(t, err)

	data, err := resp.MarshalJSON()
	require.NoError(t, err)

	// Should contain [] for empty slice, not null
	assert.Contains(t, string(data), `[]`)
	assert.NotContains(t, string(data), `"empty_slice":null`)
}

func TestProfileFast(t *testing.T) {
	SetPerformanceProfile(ProfileFast)
	defer SetPerformanceProfile(ProfileDefault)

	// ProfileFast should still handle basic operations correctly
	req := NewRequest("test_method", []any{1, 2, 3})
	data, err := req.MarshalJSON()
	require.NoError(t, err)
	assert.Contains(t, string(data), "test_method")

	// Unmarshal should work
	req2, err := DecodeRequest(data)
	require.NoError(t, err)
	assert.Equal(t, "test_method", req2.Method)
}

func TestProfileAggressive(t *testing.T) {
	SetPerformanceProfile(ProfileAggressive)
	defer SetPerformanceProfile(ProfileDefault)

	// ProfileAggressive should still produce valid JSON for valid input
	req := NewRequest("test_method", map[string]any{
		"param1": "value1",
		"param2": 42,
	})
	data, err := req.MarshalJSON()
	require.NoError(t, err)

	// Should be valid JSON
	var check map[string]any
	err = json.Unmarshal(data, &check)
	require.NoError(t, err)
	assert.Equal(t, "test_method", check["method"])
}

func TestProfileSwitchingConcurrency(_ *testing.T) {
	// Test that profile switching is thread-safe
	originalProfile := GetPerformanceProfile()
	defer SetPerformanceProfile(originalProfile)

	done := make(chan bool, 3)

	// Goroutine 1: Switch profiles
	go func() {
		for i := 0; i < 100; i++ {
			SetPerformanceProfile(ProfileDefault)
			SetPerformanceProfile(ProfileFast)
		}
		done <- true
	}()

	// Goroutine 2: Read profile
	go func() {
		for i := 0; i < 100; i++ {
			_ = GetPerformanceProfile()
		}
		done <- true
	}()

	// Goroutine 3: Use API
	go func() {
		for i := 0; i < 100; i++ {
			req := NewRequest("test", nil)
			_, _ = req.MarshalJSON()
		}
		done <- true
	}()

	// Wait for all goroutines to complete
	<-done
	<-done
	<-done
}

func TestProfileRoundTrip(t *testing.T) {
	profiles := []PerformanceProfile{
		ProfileDefault,
		ProfileCompatible,
		ProfileBalanced,
		ProfileFast,
		ProfileAggressive,
	}

	for _, profile := range profiles {
		t.Run(profile.String(), func(t *testing.T) {
			SetPerformanceProfile(profile)
			defer SetPerformanceProfile(ProfileDefault)

			// Create a request
			originalReq := NewRequest("test_method", map[string]any{
				"key": "value",
				"num": 42,
			})

			// Marshal it
			data, err := originalReq.MarshalJSON()
			require.NoError(t, err)

			// Unmarshal it
			decodedReq, err := DecodeRequest(data)
			require.NoError(t, err)

			// Verify it matches
			assert.Equal(t, originalReq.Method, decodedReq.Method)
			assert.Equal(t, originalReq.JSONRPC, decodedReq.JSONRPC)
		})
	}
}

func TestInvalidProfileIgnored(t *testing.T) {
	originalProfile := GetPerformanceProfile()
	defer SetPerformanceProfile(originalProfile)

	// Try to set an invalid profile (should be silently ignored)
	SetPerformanceProfile(PerformanceProfile(999))

	// Should still be on the original profile
	assert.Equal(t, originalProfile, GetPerformanceProfile())
}

// String returns a string representation of the performance profile for testing
func (p PerformanceProfile) String() string {
	switch p {
	case ProfileDefault:
		return "ProfileDefault"
	case ProfileCompatible:
		return "ProfileCompatible"
	case ProfileBalanced:
		return "ProfileBalanced"
	case ProfileFast:
		return "ProfileFast"
	case ProfileAggressive:
		return "ProfileAggressive"
	default:
		return "ProfileUnknown"
	}
}
