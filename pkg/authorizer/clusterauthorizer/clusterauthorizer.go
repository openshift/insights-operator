package clusterauthorizer

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/http/httpproxy"
	knet "k8s.io/apimachinery/pkg/util/net"

	"github.com/openshift/insights-operator/pkg/config/configobserver"
)

type Authorizer struct {
	configurator configobserver.Configurator
	// exposed for tests
	proxyFromEnvironment func(*http.Request) (*url.URL, error)
}

// New creates a new Authorizer, whose purpose is to auth requests for outgoing traffic.
func New(configurator configobserver.Configurator) *Authorizer {
	return &Authorizer{
		configurator:         configurator,
		proxyFromEnvironment: http.ProxyFromEnvironment,
	}
}

// Authorize adds the necessary auth header to the request, depending on the config. (BasicAuth/Token)
func (a *Authorizer) Authorize(req *http.Request) error {
	if req.Header == nil {
		req.Header = make(http.Header)
	}

	token, err := a.Token()
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	return nil
}

// NewSystemOrConfiguredProxy creates the proxy URL based on the config. (specific/default proxy)
func (a *Authorizer) NewSystemOrConfiguredProxy() func(*http.Request) (*url.URL, error) {
	// using specific proxy settings
	if c := a.configurator.Config(); c != nil {
		if len(c.HTTPConfig.HTTPProxy) > 0 || len(c.HTTPConfig.HTTPSProxy) > 0 || len(c.HTTPConfig.NoProxy) > 0 {
			proxyConfig := httpproxy.Config{
				HTTPProxy:  c.HTTPConfig.HTTPProxy,
				HTTPSProxy: c.HTTPConfig.HTTPSProxy,
				NoProxy:    c.HTTPConfig.NoProxy,
			}
			// The golang ProxyFunc seems to have NoProxy already built in
			return func(req *http.Request) (*url.URL, error) {
				return proxyConfig.ProxyFunc()(req.URL)
			}
		}
	}
	// defautl system proxy
	return knet.NewProxierWithNoProxyCIDR(a.proxyFromEnvironment)
}

func (a *Authorizer) Token() (string, error) {
	cfg := a.configurator.Config()
	if len(cfg.Token) > 0 {
		token := strings.TrimSpace(cfg.Token)
		if strings.Contains(token, "\n") || strings.Contains(token, "\r") {
			return "", fmt.Errorf("cluster authorization token is not valid: contains newlines")
		}
		if len(token) == 0 {
			return "", fmt.Errorf("cluster authorization token is empty")
		}
		return token, nil
	}
	return "", fmt.Errorf("cluster authorization token is not configured")
}
