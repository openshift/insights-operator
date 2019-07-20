package clusterauthorizer

import (
	"fmt"
	"net/http"

	"github.com/openshift/support-operator/pkg/config"
)

type Configurator interface {
	Config() *config.Controller
}

type Authorizer struct {
	configurator Configurator
}

func New(configurator Configurator) *Authorizer {
	return &Authorizer{
		configurator: configurator,
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
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.Token))
		return nil
	}
	return nil
}
