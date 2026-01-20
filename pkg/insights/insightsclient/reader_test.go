package insightsclient

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLimitedReader_Read(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		limit          int64
		bufferSize     int
		expectedRead   string
		expectedErr    error
		expectedN      int64
		expectedBytes  int
		secondRead     bool
		secondReadErr  error
	}{
		{
			name:         "within limit",
			input:        "Hello, World!",
			limit:        100,
			bufferSize:   13,
			expectedRead: "Hello, World!",
			expectedErr:  nil,
			expectedN:    87,
		},
		{
			name:         "exact limit",
			input:        "12345",
			limit:        5,
			bufferSize:   5,
			expectedRead: "12345",
			expectedErr:  nil,
			expectedN:    0,
		},
		{
			name:          "exceeds limit",
			input:         "Hello, World!",
			limit:         5,
			bufferSize:    10,
			expectedRead:  "Hello",
			expectedErr:   nil,
			expectedN:     0,
			secondRead:    true,
			secondReadErr: ErrTooLong,
		},
		{
			name:        "zero limit",
			input:       "test data",
			limit:       0,
			bufferSize:  10,
			expectedErr: ErrTooLong,
		},
		{
			name:          "buffer larger than limit",
			input:         "Hello, World! This is a long string.",
			limit:         5,
			bufferSize:    20,
			expectedErr:   nil,
			expectedN:     0,
			expectedBytes: 5,
		},
		{
			name:        "empty reader",
			input:       "",
			limit:       10,
			bufferSize:  10,
			expectedErr: io.EOF,
			expectedN:   10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lr := &LimitedReader{R: strings.NewReader(tt.input), N: tt.limit}
			buf := make([]byte, tt.bufferSize)
			n, err := lr.Read(buf)

			if tt.expectedErr != nil {
				assert.Equal(t, tt.expectedErr, err)
			} else {
				assert.NoError(t, err)
				if tt.expectedRead != "" {
					assert.Equal(t, tt.expectedRead, string(buf[:n]))
				}
				assert.Equal(t, tt.expectedN, lr.N)
				if tt.expectedBytes > 0 {
					assert.Equal(t, tt.expectedBytes, n, "should only read up to limit")
				}
			}

			if tt.secondRead {
				buf2 := make([]byte, 10)
				n2, err2 := lr.Read(buf2)
				assert.Equal(t, tt.secondReadErr, err2)
				assert.Equal(t, 0, n2)
			}
		})
	}
}

func TestLimitedReader_Read_MultipleReads(t *testing.T) {
	lr := &LimitedReader{R: strings.NewReader("0123456789"), N: 10}

	// First read
	buf1 := make([]byte, 3)
	n1, _ := lr.Read(buf1)
	assert.Equal(t, 3, n1)
	assert.Equal(t, "012", string(buf1))
	assert.Equal(t, int64(7), lr.N)

	// Second read
	buf2 := make([]byte, 4)
	n2, _ := lr.Read(buf2)
	assert.Equal(t, 4, n2)
	assert.Equal(t, "3456", string(buf2))
	assert.Equal(t, int64(3), lr.N)

	// Third read (partial)
	buf3 := make([]byte, 5)
	n3, _ := lr.Read(buf3)
	assert.Equal(t, 3, n3)
	assert.Equal(t, "789", string(buf3[:n3]))
	assert.Equal(t, int64(0), lr.N)

	// Fourth read (should error)
	buf4 := make([]byte, 5)
	n4, err4 := lr.Read(buf4)
	assert.Equal(t, ErrTooLong, err4)
	assert.Equal(t, 0, n4)
}

func TestLimitedReader_Integration_WithIOCopy(t *testing.T) {
	input := "Lorem ipsum dolor sit amet, consectetur adipiscing elit"
	reader := strings.NewReader(input)
	limit := int64(20)
	lr := LimitReader(reader, limit)

	var output bytes.Buffer
	n, err := io.Copy(&output, lr)

	assert.Equal(t, ErrTooLong, err, "should stop with ErrTooLong")
	assert.Equal(t, limit, n, "should copy exactly limit bytes")
	assert.Equal(t, input[:20], output.String())
}
