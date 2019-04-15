package clusterauthorizer

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"k8s.io/klog"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	"github.com/openshift/support-operator/pkg/authorizer"
)

type authorizerResult struct {
	enabled bool
	message string
	at      time.Time

	endpoint string
	token    string
	username string
	password string
}

type Authorizer struct {
	client kubernetes.Interface

	lock   sync.Mutex
	result authorizerResult
}

func New(client kubernetes.Interface) *Authorizer {
	return &Authorizer{
		client: client,
	}
}

func (a *Authorizer) Authorize(req *http.Request) error {
	result := a.latestResult()
	if !result.enabled {
		return authorizer.Error{Err: fmt.Errorf("no authorization info is currently available")}
	}
	if len(result.endpoint) > 0 {
		endpoint, err := url.Parse(result.endpoint)
		if err != nil {
			return fmt.Errorf("configured endpoint is not a valid URL: %v", err)
		}
		req.Host = endpoint.Host
		req.URL = endpoint
	}
	if len(result.username) > 0 || len(result.password) > 0 {
		req.SetBasicAuth(result.username, result.password)
		return nil
	}
	if len(result.token) > 0 {
		if req.Header == nil {
			req.Header = make(http.Header)
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", result.token))
		return nil
	}
	return nil
}

func (a *Authorizer) Enabled() (bool, string) {
	result := a.latestResult()
	if result.enabled {
		return true, ""
	}
	if len(result.message) == 0 {
		return false, "Reports will not be uploaded, no credentials specified."
	}
	return false, result.message
}

func (a *Authorizer) latestResult() authorizerResult {
	a.lock.Lock()
	defer a.lock.Unlock()
	return a.result
}

func (a *Authorizer) Run(ctx context.Context, baseInterval time.Duration) {
	wait.Until(func() {
		for {
			var interval time.Duration
			if err := a.refresh(); err != nil {
				klog.Errorf("Unable to refresh authorization secret: %v", err)
				interval = baseInterval / 2
			} else {
				interval = baseInterval * 5
			}
			time.Sleep(interval)
		}
	}, 1*time.Minute, ctx.Done())
}

func (a *Authorizer) refresh() error {
	var err error
	var result authorizerResult

	secret, err := a.client.CoreV1().Secrets("openshift-config").Get("support", metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			klog.V(4).Infof("Support secret does not exist, reporting is disabled")
			result.message = "Reporting is disabled until the openshift-config/support secret is created with username and password fields"
			err = nil
		} else if errors.IsForbidden(err) {
			klog.V(2).Infof("Operator does not have permission to check support secret: %v", err)
			err = nil
		} else {
			err = fmt.Errorf("could not check support secret: %v", err)
		}
	} else {
		if username, ok := secret.Data["username"]; ok {
			result.username = string(username)
		}
		if password, ok := secret.Data["password"]; ok {
			result.password = string(password)
		}

		if endpoint, ok := secret.Data["endpoint"]; ok {
			result.endpoint = string(endpoint)
			result.enabled = len(result.endpoint) > 0
			if !result.enabled {
				result.message = "Reporting has been disabled via configuration"
			}
		} else {
			result.enabled = true
		}
	}

	// TODO: when endpoint supports token
	// secret, err = a.client.CoreV1().Secrets("kube-system").Get("coreos-pull-secret", metav1.GetOptions{})
	// if err != nil {
	// 	if !errors.IsNotFound(err) && !errors.IsForbidden(err) {
	// 		err = fmt.Errorf("could not check cloud token: %v", err)
	// 	} else {
	// 		klog.V(4).Infof("Unable to check system pull secret: %v", err)
	// 	}
	// } else {
	// 	if data := secret.Data[".dockerconfigjson"]; len(data) > 0 {
	// 		var pullSecret serializedAuthMap
	// 		if err := json.Unmarshal(data, &pullSecret); err != nil {
	// 			klog.Errorf("Unable to unmarshal system pull secret: %v", err)
	// 		}
	// 		if auth, ok := pullSecret.Auths["cloud.openshift.com"]; ok && len(auth.Auth) > 0 {
	// 			klog.V(4).Info("Found cloud.openshift.com token")
	// 			result.token = auth.Auth
	// 			result.enabled = true
	// 		}
	// 	}
	// }

	if result.enabled {
		result.at = time.Now()
	}

	a.lock.Lock()
	defer a.lock.Unlock()
	a.result = result

	return err
}

type serializedAuthMap struct {
	Auths map[string]serializedAuth `json:"auths"`
}

type serializedAuth struct {
	Auth string `json:"auth"`
}
