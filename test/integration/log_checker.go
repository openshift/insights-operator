package integration

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

type (
	LogCheck struct {
		test           *testing.T
		clientset      *kubernetes.Clientset
		firstCheck     time.Time
		lastCheck      time.Time
		since          time.Time
		logOptions     corev1.PodLogOptions
		interval       time.Duration
		timeout        time.Duration
		namespace      string
		podName        string
		searching      string
		sinceLastCheck bool
		failFast       bool
		Err            error
		Result         string
	}
)

const ALLPODS string = ""

func (lc *LogCheck) Interval(interval time.Duration) *LogCheck {
	lc.interval = interval
	return lc
}

func (lc *LogCheck) FailFast(failFast ...bool) *LogCheck {
	if len(failFast) == 0 {
		lc.failFast = true
	}
	lc.failFast = failFast[0]
	return lc
}

func (lc *LogCheck) Timeout(timeout time.Duration) *LogCheck {
	lc.timeout = timeout
	return lc
}

func (lc *LogCheck) Since(since time.Time) *LogCheck {
	lc.since = since
	return lc
}

func (lc *LogCheck) SinceNow() *LogCheck {
	lc.since = time.Now()
	return lc
}

func (lc *LogCheck) EnableSinceLastCheck() *LogCheck {
	lc.sinceLastCheck = true
	return lc
}

func (lc *LogCheck) DisableSinceLastCheck() *LogCheck {
	lc.sinceLastCheck = true
	return lc
}

func (lc *LogCheck) Searching(s string) *LogCheck {
	lc.searching = s
	return lc
}

func (lc *LogCheck) Namespace(s string) *LogCheck {
	lc.namespace = s
	return lc
}

func (lc *LogCheck) PodName(s string) *LogCheck {
	// specify pod name, if it's left empty, all pods in given namespace will be checked
	lc.podName = s
	return lc
}

func (lc *LogCheck) Search(s string) *LogCheck {
	return lc.Searching(s).Execute()
}

func (lc *LogCheck) CheckPodLogs(podName string, logOptions *corev1.PodLogOptions, r *regexp.Regexp) error {
	t := lc.test
	pod, err := lc.clientset.CoreV1().Pods(lc.namespace).Get(context.Background(), podName, metav1.GetOptions{})
	if err != nil {
		panic(err.Error())
	}
	return wait.PollImmediate(lc.interval, lc.timeout, func() (bool, error) {
		req := lc.clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, logOptions)
		podLogs, err := req.Stream(context.Background())
		if err != nil {
			return false, nil
		}
		defer podLogs.Close()

		buf := new(bytes.Buffer)
		lc.lastCheck = time.Now()
		_, err = io.Copy(buf, podLogs)
		if err != nil {
			lc.Err = err
			if lc.failFast {
				t.Fatal(err.Error())
			}
		}
		log := buf.String()

		lc.Result = r.FindString(log) //strings.Contains(log, message)
		if lc.Result == "" {
			return false, nil
		}

		t.Logf("%s found\n", lc.Result)
		return true, nil
	})
}

func (lc *LogCheck) Execute() *LogCheck {
	t := lc.test
	kubeClient := lc.clientset
	if lc.namespace == "" {
		lc.namespace = "openshift-insights"
	}
	namespace := lc.namespace
	lc.Result = ""
	startOfAges := time.Time{}
	now := time.Now()

	if lc.firstCheck == startOfAges {
		lc.firstCheck = now
	}
	r := regexp.MustCompile(`(?m)` + lc.searching)
	logOptions := &corev1.PodLogOptions{}
	if lc.sinceLastCheck && lc.lastCheck != startOfAges {
		last := metav1.NewTime(lc.lastCheck)
		logOptions = &corev1.PodLogOptions{SinceTime: &last}
	} else {
		since := metav1.NewTime(lc.since)
		logOptions = &corev1.PodLogOptions{SinceTime: &since}
	}
	if lc.podName != ALLPODS {
		lc.Err = lc.CheckPodLogs(lc.podName, logOptions, r)
		return lc
	}
	newPods, err := kubeClient.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
	resultError := fmt.Errorf("Couldn't find %s", lc.searching)
	for _, newPod := range newPods.Items {
		err = lc.CheckPodLogs(newPod.Name, logOptions, r)
		if err == nil {
			resultError = nil
		}
	}
	if lc.failFast && resultError != nil {
		t.Error(resultError)
	}
	lc.Err = resultError
	return lc
}

func logChecker(t *testing.T, clientset *kubernetes.Clientset) *LogCheck {
	defaults := &LogCheck{
		interval:   time.Second,
		logOptions: corev1.PodLogOptions{},
		timeout:    5 * time.Minute,
		failFast:   true,
		test:       t,
		clientset:  clientset,
	}
	return defaults
}
