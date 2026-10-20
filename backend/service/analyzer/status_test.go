package analyzer

import (
	"context"
	"testing"

	"ai-disk-cleanner/backend/data/models/setting"
)

type statusSettingsStore struct {
	settings []setting.Setting
}

func (store statusSettingsStore) ListSettings(context.Context) ([]setting.Setting, error) {
	return store.settings, nil
}

func TestGetStatusReportsAPIConfigurationReadiness(t *testing.T) {
	tests := []struct {
		name     string
		settings []setting.Setting
		ready    bool
		code     string
	}{
		{name: "missing", settings: nil, ready: false, code: "config_missing"},
		{name: "configured", settings: legacyAPISettings("https://example.test/v1"), ready: true, code: "ready"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, err := newService(statusSettingsStore{settings: test.settings}).GetStatus(context.Background())
			if err != nil {
				t.Fatalf("GetStatus() error = %v", err)
			}
			if status.Provider != "api" || status.Ready != test.ready || status.Code != test.code {
				t.Fatalf("GetStatus() = %#v", status)
			}
		})
	}
}

func TestGetStatusUsesReadinessProbeWithoutTestingModelAccess(t *testing.T) {
	settings := []setting.Setting{
		{Key: "llm.provider", Value: "codex"},
		{Key: "codex.executable", Value: "/path/codex"},
		{Key: "codex.model", Value: ""},
		{Key: "codex.timeout-seconds", Value: "600"},
	}
	service := newService(statusSettingsStore{settings: settings})
	probeCalls := 0
	service.codexStatusProbe = func(context.Context, *codexConfig) (ProviderStatus, error) {
		probeCalls++
		return ProviderStatus{Ready: true, Code: "ready", Version: "0.162.1"}, nil
	}

	status, err := service.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if probeCalls != 1 || status.Provider != "codex" || !status.Ready || status.Code != "ready" || status.Version != "0.162.1" {
		t.Fatalf("GetStatus() = %#v, probe calls = %d", status, probeCalls)
	}
}
