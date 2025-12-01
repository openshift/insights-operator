package clusterauthorizer

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"golang.org/x/net/http/httpproxy"

	"github.com/openshift/insights-operator/pkg/config"
)

// nonCachedProxyFromEnvironment creates Proxier if Proxy is set. It uses always fresh Env
func nonCachedProxyFromEnvironment() func(*http.Request) (*url.URL, error) {
	return func(req *http.Request) (*url.URL, error) {
		return httpproxy.FromEnvironment().ProxyFunc()(req.URL)
	}
}

func Test_Proxy(tt *testing.T) {
	testCases := []struct {
		Name        string
		EnvValues   map[string]interface{}
		RequestURL  string
		ProxyConfig config.Proxy
		ProxyURL    string
	}{
		{
			Name:       "No env set, no specific proxy",
			EnvValues:  map[string]interface{}{"HTTP_PROXY": nil},
			RequestURL: "http://google.com",
			ProxyURL:   "",
		},
		{
			Name:       "Env set, no specific proxy",
			EnvValues:  map[string]interface{}{"HTTP_PROXY": "proxy.to"},
			RequestURL: "http://google.com",
			ProxyURL:   "http://proxy.to",
		},
		{
			Name:       "Env set with HTTPS, no specific proxy",
			EnvValues:  map[string]interface{}{"HTTPS_PROXY": "secproxy.to"},
			RequestURL: "https://google.com",
			ProxyURL:   "http://secproxy.to",
		},
		{
			Name:        "Env not set, specific proxy set",
			EnvValues:   map[string]interface{}{"HTTP_PROXY": nil},
			RequestURL:  "http://google.com",
			ProxyConfig: config.Proxy{HTTPProxy: "specproxy.to"},
			ProxyURL:    "http://specproxy.to",
		},
		{
			Name:        "Env set, specific proxy set http",
			EnvValues:   map[string]interface{}{"HTTP_PROXY": "envproxy.to"},
			RequestURL:  "http://google.com",
			ProxyConfig: config.Proxy{HTTPProxy: "specproxy.to"},
			ProxyURL:    "http://specproxy.to",
		},
		{
			Name:        "Env set, specific proxy set https",
			EnvValues:   map[string]interface{}{"HTTPS_PROXY": "envsecproxy.to"},
			RequestURL:  "https://google.com",
			ProxyConfig: config.Proxy{HTTPSProxy: "specsecproxy.to"},
			ProxyURL:    "http://specsecproxy.to",
		},
		{
			Name:        "Env set, specific proxy set noproxy, request without noproxy",
			EnvValues:   map[string]interface{}{"HTTPS_PROXY": "envsecproxy.to", "NO_PROXY": "envnoproxy.to"},
			RequestURL:  "https://google.com",
			ProxyConfig: config.Proxy{HTTPSProxy: "specsecproxy.to", NoProxy: "specnoproxy.to"},
			ProxyURL:    "http://specsecproxy.to",
		},
		{
			Name:        "Env set, specific proxy set noproxy, request with noproxy",
			EnvValues:   map[string]interface{}{"HTTPS_PROXY": "envsecproxy.to", "NO_PROXY": "envnoproxy.to"},
			RequestURL:  "https://specnoproxy.to",
			ProxyConfig: config.Proxy{HTTPSProxy: "specsecproxy.to", NoProxy: "specnoproxy.to"},
			ProxyURL:    "",
		},
	}
	for _, tcase := range testCases {
		tc := tcase
		tt.Run(tc.Name, func(t *testing.T) {
			for k, v := range tc.EnvValues {
				// do not use parallel here
				defer SafeRestoreEnv(k)() // nolint: gocritic
				// nil will indicate the need to unset Env
				if v != nil {
					vv := v.(string)
					os.Setenv(k, vv)
				} else {
					os.Unsetenv(k)
				}
			}

			secretConfigurator := &config.MockSecretConfigurator{Conf: &config.Controller{}}
			configurator := config.NewMockConfigMapConfigurator(&config.InsightsConfiguration{
				Proxy: tc.ProxyConfig,
			})
			a := Authorizer{
				proxyFromEnvironment: nonCachedProxyFromEnvironment(),
				secretConfigurator:   secretConfigurator,
				configurator:         configurator,
			}
			p := a.NewSystemOrConfiguredProxy()
			req := httptest.NewRequest("GET", tc.RequestURL, http.NoBody)
			urlRec, err := p(req)

			if err != nil {
				t.Fatalf("unexpected err %s", err)
			}
			if (tc.ProxyURL == "" && urlRec != nil) ||
				(len(tc.ProxyURL) > 0 && (urlRec == nil || tc.ProxyURL != urlRec.String())) {
				t.Fatalf("Unexpected value of Proxy Url. Test %s Expected Url %s Received Url %s", tc.Name, tc.ProxyURL, urlRec)
			}
		})
	}
}

func SafeRestoreEnv(key string) func() {
	originalVal, wasSet := os.LookupEnv(key)
	return func() {
		if !wasSet {
			fmt.Printf("unsetting key %s", key)
			os.Unsetenv(key)
		} else {
			fmt.Printf("restoring key %s", key)
			os.Setenv(key, originalVal)
		}
	}
}

func TestNew(t *testing.T) {
	secretConfigurator := &config.MockSecretConfigurator{Conf: &config.Controller{}}
	configurator := config.NewMockConfigMapConfigurator(&config.InsightsConfiguration{})

	auth := New(secretConfigurator, configurator)

	if auth == nil {
		t.Fatal("Expected non-nil Authorizer")
	}
	if auth.secretConfigurator != secretConfigurator {
		t.Error("secretConfigurator not set correctly")
	}
	if auth.configurator != configurator {
		t.Error("configurator not set correctly")
	}
	if auth.proxyFromEnvironment == nil {
		t.Error("proxyFromEnvironment should be set to http.ProxyFromEnvironment")
	}
}

func TestToken_ValidToken(t *testing.T) {
	tests := []struct {
		name          string
		token         string
		expectedToken string
		expectError   bool
		errorContains string
	}{
		{
			name:          "valid token",
			token:         "valid-token-12345",
			expectedToken: "valid-token-12345",
			expectError:   false,
		},
		{
			name:          "valid token with whitespace trimmed",
			token:         "  valid-token-with-spaces  ",
			expectedToken: "valid-token-with-spaces",
			expectError:   false,
		},
		{
			name:          "token with newline",
			token:         "invalid\ntoken",
			expectedToken: "",
			expectError:   true,
			errorContains: "contains newlines",
		},
		{
			name:          "empty token",
			token:         "",
			expectedToken: "",
			expectError:   true,
			errorContains: "not configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secretConfigurator := &config.MockSecretConfigurator{
				Conf: &config.Controller{
					Token: tt.token,
				},
			}
			configurator := config.NewMockConfigMapConfigurator(&config.InsightsConfiguration{})

			auth := Authorizer{
				secretConfigurator: secretConfigurator,
				configurator:       configurator,
			}

			token, err := auth.Token()

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error containing %q but got nil", tt.errorContains)
				} else if !contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing %q but got %q", tt.errorContains, err.Error())
				}
				if token != "" {
					t.Errorf("Expected empty token on error but got %q", token)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
				if token != tt.expectedToken {
					t.Errorf("Expected token %q but got %q", tt.expectedToken, token)
				}
			}
		})
	}
}

func TestAuthorize(t *testing.T) {
	tests := []struct {
		name               string
		token              string
		expectError        bool
		errorContains      string
		expectedAuthHeader string
	}{
		{
			name:               "success",
			token:              "test-bearer-token",
			expectError:        false,
			expectedAuthHeader: "Bearer test-bearer-token",
		},
		{
			name:          "token with newline",
			token:         "invalid\ntoken",
			expectError:   true,
			errorContains: "contains newlines",
		},
		{
			name:          "empty token",
			token:         "",
			expectError:   true,
			errorContains: "not configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secretConfigurator := &config.MockSecretConfigurator{
				Conf: &config.Controller{
					Token: tt.token,
				},
			}
			configurator := config.NewMockConfigMapConfigurator(&config.InsightsConfiguration{})

			auth := Authorizer{
				secretConfigurator: secretConfigurator,
				configurator:       configurator,
			}

			req := httptest.NewRequest("GET", "http://example.com", http.NoBody)
			err := auth.Authorize(req)

			if tt.expectError {
				if err == nil {
					t.Fatal("Expected error but got nil")
				}
				if !contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing %q but got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error but got: %v", err)
				}
				authHeader := req.Header.Get("Authorization")
				if authHeader != tt.expectedAuthHeader {
					t.Errorf("Expected Authorization header %q but got %q", tt.expectedAuthHeader, authHeader)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
