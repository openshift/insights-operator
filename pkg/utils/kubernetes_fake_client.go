package utils

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/testing"
)

func AddObjectsToClientSet[C ~[]T, T runtime.Object](cli testing.FakeClient, obj C) error {
	for i := range obj {
		o := obj[i]
		err := cli.Tracker().Add(o)
		if err != nil {
			return err
		}
	}
	return nil
}
