package config

import (
	"testing"
)

func TestWebRTCConfigMerge(t *testing.T) {
	// Create base config with defaults
	baseConfig := DefaultWebRTCConfig()

	// Create other config with different values
	otherConfig := &WebRTCConfig{
		ICEServers: []ICEServerConfig{
			{
				URLs: []string{"stun:custom.stun.server:19302"},
			},
		},
		SignalingPath: "/custom-ws",
		EnableTURN:    true,
		TURNConfig: TURNConfig{
			Host:     "custom.turn.server",
			Port:     3479,
			Username: "testuser",
			Password: "testpass",
			Realm:    "custom-realm",
		},
	}

	// Merge the configs
	err := baseConfig.Merge(otherConfig)
	if err != nil {
		t.Fatalf("Merge failed: %v", err)
	}

	// Verify that the merge worked correctly
	if baseConfig.SignalingPath != "/custom-ws" {
		t.Errorf("Expected SignalingPath to be '/custom-ws', got '%s'", baseConfig.SignalingPath)
	}

	if len(baseConfig.ICEServers) != 1 {
		t.Errorf("Expected 1 ICE server, got %d", len(baseConfig.ICEServers))
	}

	if baseConfig.ICEServers[0].URLs[0] != "stun:custom.stun.server:19302" {
		t.Errorf("Expected custom STUN server, got '%s'", baseConfig.ICEServers[0].URLs[0])
	}

	if !baseConfig.EnableTURN {
		t.Error("Expected EnableTURN to be true")
	}

	if baseConfig.TURNConfig.Host != "custom.turn.server" {
		t.Errorf("Expected TURN host to be 'custom.turn.server', got '%s'", baseConfig.TURNConfig.Host)
	}

	if baseConfig.TURNConfig.Port != 3479 {
		t.Errorf("Expected TURN port to be 3479, got %d", baseConfig.TURNConfig.Port)
	}

	if baseConfig.TURNConfig.Username != "testuser" {
		t.Errorf("Expected TURN username to be 'testuser', got '%s'", baseConfig.TURNConfig.Username)
	}

	if baseConfig.TURNConfig.Password != "testpass" {
		t.Errorf("Expected TURN password to be 'testpass', got '%s'", baseConfig.TURNConfig.Password)
	}

	if baseConfig.TURNConfig.Realm != "custom-realm" {
		t.Errorf("Expected TURN realm to be 'custom-realm', got '%s'", baseConfig.TURNConfig.Realm)
	}
}

func TestWebRTCConfigMergeWithDefaults(t *testing.T) {
	// Create base config with defaults
	baseConfig := DefaultWebRTCConfig()

	// Create other config with only some values (others should remain default)
	otherConfig := &WebRTCConfig{
		SignalingPath: "/custom-ws",
		EnableTURN:    false, // This should not override since it's the same as default
		TURNConfig: TURNConfig{
			Host:     "localhost", // This should not override since it's the default
			Port:     3478,        // This should not override since it's the default
			Username: "newuser",   // This should override
			Password: "newpass",   // This should override
			Realm:    "bdwind",    // This should not override since it's the default
		},
	}

	// Store original values for comparison
	originalICEServers := make([]ICEServerConfig, len(baseConfig.ICEServers))
	copy(originalICEServers, baseConfig.ICEServers)

	// Merge the configs
	err := baseConfig.Merge(otherConfig)
	if err != nil {
		t.Fatalf("Merge failed: %v", err)
	}

	// Verify that the merge worked correctly
	if baseConfig.SignalingPath != "/custom-ws" {
		t.Errorf("Expected SignalingPath to be '/custom-ws', got '%s'", baseConfig.SignalingPath)
	}

	// ICE servers should remain the same since otherConfig has empty ICEServers
	if len(baseConfig.ICEServers) != len(originalICEServers) {
		t.Errorf("Expected %d ICE servers, got %d", len(originalICEServers), len(baseConfig.ICEServers))
	}

	// EnableTURN should remain false (default)
	if baseConfig.EnableTURN {
		t.Error("Expected EnableTURN to remain false")
	}

	// TURN config should be partially updated
	if baseConfig.TURNConfig.Host != "localhost" {
		t.Errorf("Expected TURN host to remain 'localhost', got '%s'", baseConfig.TURNConfig.Host)
	}

	if baseConfig.TURNConfig.Port != 3478 {
		t.Errorf("Expected TURN port to remain 3478, got %d", baseConfig.TURNConfig.Port)
	}

	if baseConfig.TURNConfig.Username != "newuser" {
		t.Errorf("Expected TURN username to be 'newuser', got '%s'", baseConfig.TURNConfig.Username)
	}

	if baseConfig.TURNConfig.Password != "newpass" {
		t.Errorf("Expected TURN password to be 'newpass', got '%s'", baseConfig.TURNConfig.Password)
	}

	if baseConfig.TURNConfig.Realm != "bdwind" {
		t.Errorf("Expected TURN realm to remain 'bdwind', got '%s'", baseConfig.TURNConfig.Realm)
	}
}

func TestWebRTCConfigMergeInvalidType(t *testing.T) {
	baseConfig := DefaultWebRTCConfig()

	// Try to merge with a different config type (WebServerConfig)
	otherConfig := DefaultWebServerConfig()

	err := baseConfig.Merge(otherConfig)
	if err == nil {
		t.Error("Expected error when merging different config types")
	}

	expectedError := "cannot merge different config types"
	if err.Error() != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, err.Error())
	}
}
