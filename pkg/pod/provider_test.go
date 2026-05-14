package pod

import (
	"testing"
)

func TestNewProvider(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		apiKey       string
		wantErr      bool
		errMsg       string
		checkType    func(Provider) bool
	}{
		{
			name:         "create printful provider",
			providerName: "printful",
			apiKey:       "test_key",
			wantErr:      false,
			checkType: func(p Provider) bool {
				_, ok := p.(*PrintfulProvider)
				return ok
			},
		},
		{
			name:         "unsupported provider",
			providerName: "unknown",
			apiKey:       "test_key",
			wantErr:      true,
			errMsg:       "unsupported provider: unknown",
		},
		{
			name:         "empty provider name",
			providerName: "",
			apiKey:       "test_key",
			wantErr:      true,
			errMsg:       "unsupported provider: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewProvider(tt.providerName, tt.apiKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewProvider() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if err.Error() != tt.errMsg {
					t.Errorf("NewProvider() error message = %v, want %v", err.Error(), tt.errMsg)
				}
			}
			if !tt.wantErr {
				if provider == nil {
					t.Error("NewProvider() returned nil provider")
					return
				}
				if tt.checkType != nil && !tt.checkType(provider) {
					t.Errorf("NewProvider() returned wrong provider type")
				}
				// Verify provider implements the interface
				if provider.Name() == "" {
					t.Error("Provider.Name() returned empty string")
				}
			}
		})
	}
}

func TestPrintfulProvider_Name(t *testing.T) {
	provider := NewPrintfulProvider("test_key")
	if provider.Name() != "printful" {
		t.Errorf("PrintfulProvider.Name() = %v, want 'printful'", provider.Name())
	}
}
