package bot

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseCommand(t *testing.T) {
	tests := []struct{ text, cmd, arg string }{
		{"/start K7x9QpLm2aBc", "/start", "K7x9QpLm2aBc"},
		{"/start", "/start", ""},
		{"  /START@cargoflow_bot  abc ", "/start", "abc"},
		{"/stop", "/stop", ""},
		{"привет", "привет", ""},
	}
	for _, tc := range tests {
		cmd, arg := parseCommand(tc.text)
		assert.Equal(t, tc.cmd, cmd, tc.text)
		assert.Equal(t, tc.arg, arg, tc.text)
	}
}
