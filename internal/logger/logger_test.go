package logger

import "testing"

func TestNop(_ *testing.T) {
	Nop()
}

func TestLevelFlagSet(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLevel int
		wantErr   bool
	}{
		{"true_value", "true", 1, false},
		{"false_value", "false", 0, false},
		{"integer_value", "5", 5, false},
		{"invalid_value", "abc", -1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level := -1
			f := &LevelFlag{Level: &level}
			err := f.Set(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Set() error = %v, wantErr %v", err, tt.wantErr)
			}
			if level != tt.wantLevel {
				t.Errorf("Set() level = %v, want %v", level, tt.wantLevel)
			}
		})
	}
}
func TestLevelFlagString(t *testing.T) {
	val := 5
	tests := []struct {
		name  string
		level *int
		want  string
	}{
		{"nil_level", nil, "0"},
		{"valid_level", &val, "5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &LevelFlag{Level: tt.level}
			if got := f.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}
func TestLevelFlagIsBoolFlag(t *testing.T) {
	f := &LevelFlag{}
	if got := f.IsBoolFlag(); !got {
		t.Errorf("IsBoolFlag() = %v, want true", got)
	}
}

type testWriter struct{}

func (testWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func TestSetup(t *testing.T) {
	tests := []struct {
		name  string
		level int
	}{
		{"level_0", 0},
		{"level_1", 1},
		{"level_2", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			l := Setup(tt.level, testWriter{})
			l.Info().Msg("test message")
		})
	}
}
