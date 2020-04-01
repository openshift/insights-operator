package clusterauthorizer

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/openshift/insights-operator/pkg/config"
	"golang.org/x/net/http/httpproxy"
)

// nonCachedProxyFromEnvironment creates Proxier if Proxy is set. It uses always fresh Env
func nonCachedProxyFromEnvironment() func(*http.Request) (*url.URL, error) {
	return func(req *http.Request) (*url.URL, error) {
		return httpproxy.FromEnvironment().ProxyFunc()(req.URL)
	}
}

func TestProxy(tt *testing.T) {
	testCases := []struct {
		Name       string
		EnvValues  map[string]interface{}
		RequestURL string
		HttpConfig config.HTTPConfig
		ProxyURL   string
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
			Name:       "Env not set, specific proxy set",
			EnvValues:  map[string]interface{}{"HTTP_PROXY": nil},
			RequestURL: "http://google.com",
			HttpConfig: config.HTTPConfig{HTTPProxy: "specproxy.to"},
			ProxyURL:   "http://specproxy.to",
		},
		{
			Name:       "Env set, specific proxy set http",
			EnvValues:  map[string]interface{}{"HTTP_PROXY": "envproxy.to"},
			RequestURL: "http://google.com",
			HttpConfig: config.HTTPConfig{HTTPProxy: "specproxy.to"},
			ProxyURL:   "http://specproxy.to",
		},
		{
			Name:       "Env set, specific proxy set https",
			EnvValues:  map[string]interface{}{"HTTPS_PROXY": "envsecproxy.to"},
			RequestURL: "https://google.com",
			HttpConfig: config.HTTPConfig{HTTPProxy: "specsecproxy.to"},
			ProxyURL:   "http://specsecproxy.to",
		},
		{
			Name:       "Env set, specific proxy set noproxy, request without noproxy",
			EnvValues:  map[string]interface{}{"HTTPS_PROXY": "envsecproxy.to", "NO_PROXY": "envnoproxy.to"},
			RequestURL: "https://google.com",
			HttpConfig: config.HTTPConfig{HTTPProxy: "specsecproxy.to", NoProxy: "specnoproxy.to"},
			ProxyURL:   "http://specsecproxy.to",
		},
		{
			Name:       "Env set, specific proxy set noproxy, request with noproxy",
			EnvValues:  map[string]interface{}{"HTTPS_PROXY": "envsecproxy.to", "NO_PROXY": "envnoproxy.to"},
			RequestURL: "https://specnoproxy.to",
			HttpConfig: config.HTTPConfig{HTTPProxy: "specsecproxy.to", NoProxy: "specnoproxy.to"},
			ProxyURL:   "",
		},
	}
	for _, tcase := range testCases {
		tc := tcase
		tt.Run(tc.Name, func(t *testing.T) {
			for k, v := range tc.EnvValues {
				defer SafeRestoreEnv(k)()
				// nil will indicate the need to unset Env
				if v != nil {
					vv := v.(string)
					os.Setenv(k, vv)
				} else {
					os.Unsetenv(k)
				}
			}

			co2 := &testConfig{config: &config.Controller{HTTPConfig: tc.HttpConfig}}
			a := Authorizer{proxyFromEnvironment: nonCachedProxyFromEnvironment(), configurator: co2}
			p := a.NewSystemOrConfiguredProxy()
			req := httptest.NewRequest("GET", tc.RequestURL, nil)
			url, err := p(req)

			if err != nil {
				t.Fatalf("unexpected err %s", err)
			}
			if (tc.ProxyURL == "" && url != nil) ||
				(len(tc.ProxyURL) > 0 && (url == nil || tc.ProxyURL != url.String())) {
				t.Fatalf("Unexpected value of Proxy Url. Test %s Expected Url %s Received Url %s", tc.Name, tc.ProxyURL, url)
			}
		})
	}
}

type testConfig struct {
	config *config.Controller
}

func (t *testConfig) Config() *config.Controller {
	return t.config
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
