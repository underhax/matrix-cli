package main

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestPrintFatalError(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	tests := []struct {
		err            error
		jsonMarshalErr error
		name           string
		wantOutput     string
		args           []string
	}{
		{
			name:           "json_mode_error",
			args:           []string{"cmd", flagJSON},
			err:            errors.New("json error"),
			jsonMarshalErr: errors.New("mock marshal error"),
			wantOutput:     `{"level":"fatal","error":"json error"}` + "\n",
		},
		{
			name:       "json_mode_success",
			args:       []string{"cmd", flagJSON},
			err:        errors.New("json error"),
			wantOutput: `"level":"fatal"`,
		},
		{
			name:       "text_mode",
			args:       []string{"cmd_text"},
			err:        errors.New("text error"),
			wantOutput: "Error: text error\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Args = tt.args
			origJSON := jsonMarshal
			if tt.jsonMarshalErr != nil {
				jsonMarshal = func(_ any) ([]byte, error) {
					return nil, tt.jsonMarshalErr
				}
			}
			defer func() { jsonMarshal = origJSON }()

			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("failed to create pipe: %v", err)
			}
			origStderr := os.Stderr
			os.Stderr = w
			defer func() { os.Stderr = origStderr }()

			printFatalError(tt.err)
			if closeErr := w.Close(); closeErr != nil {
				t.Fatalf("failed to close pipe: %v", closeErr)
			}

			out, readErr := io.ReadAll(r)
			if readErr != nil {
				t.Fatalf("failed to read pipe: %v", readErr)
			}
			if !strings.Contains(string(out), tt.wantOutput) {
				t.Errorf("expected output to contain %q, got %q", tt.wantOutput, string(out))
			}
		})
	}
}
