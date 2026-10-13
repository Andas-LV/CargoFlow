package grpcx

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestListenAddr(t *testing.T) {
	tests := map[string]string{
		"localhost:50051": ":50051",
		"auth:50051":      ":50051",
		":50051":          ":50051",
		"no-port":         "no-port",
	}
	for in, want := range tests {
		assert.Equal(t, want, ListenAddr(in), in)
	}
}
