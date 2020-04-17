package clusterauthorizer

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/openshift/insights-operator/pkg/config"
	"golang.org/x/net/http/httpproxy"
	knet "k8s.io/apimachinery/pkg/util/net"
)

type Configurator interface {
	Config() *config.Controller
}

type Authorizer struct {
	configurator Configurator
	// exposed for tests
	proxyFromEnvironment func(*http.Request) (*url.URL, error)
}

func New(configurator Configurator) *Authorizer {
	return &Authorizer{
		configurator:         configurator,
		proxyFromEnvironment: http.ProxyFromEnvironment,
	}
}

func (a *Authorizer) Authorize(req *http.Request) error {
	cfg := a.configurator.Config()
	if len(cfg.Username) > 0 || len(cfg.Password) > 0 {
		req.SetBasicAuth(cfg.Username, cfg.Password)
		return nil
	}
	if len(cfg.Token) > 0 {
		if req.Header == nil {
			req.Header = make(http.Header)
		}
		token := strings.TrimSpace(cfg.Token)
		if strings.Contains(token, "\n") || strings.Contains(token, "\r") {
			return fmt.Errorf("cluster authorization token is not valid: contains newlines")
		}
		if len(token) == 0 {
			return fmt.Errorf("cluster authorization token is empty")
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		return nil
	}
	return nil
}

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
