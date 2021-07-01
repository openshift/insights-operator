package utils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_sumErrors(t *testing.T) {
	err := SumErrors([]error{})
	assert.NoError(t, err)

	err = SumErrors([]error{
		fmt.Errorf("test error"),
	})
	assert.EqualError(t, err, "test error")

	err = SumErrors([]error{
		fmt.Errorf("error 1"),
		fmt.Errorf("error 2"),
		fmt.Errorf("error 3"),
	})
	assert.EqualError(t, err, "error 1, error 2, error 3")

	err = SumErrors([]error{
		fmt.Errorf("error 3"),
		fmt.Errorf("error 3"),
		fmt.Errorf("error 2"),
		fmt.Errorf("error 1"),
		fmt.Errorf("error 5"),
	})
	assert.EqualError(t, err, "error 1, error 2, error 3, error 5")
}
