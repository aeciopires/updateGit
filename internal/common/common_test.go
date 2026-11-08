package common

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
)

func TestFindExecutable(t *testing.T) {
	dir := t.TempDir()

	execFile := filepath.Join(dir, "exec.sh")
	if err := os.WriteFile(execFile, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	plainFile := filepath.Join(dir, "plain.txt")
	if err := os.WriteFile(plainFile, []byte("text"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"executable file", execFile, true},
		{"non executable file", plainFile, false},
		{"directory", dir, false},
		{"missing file", filepath.Join(dir, "missing"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FindExecutable(tt.path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("FindExecutable(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestStringToEnvVar(t *testing.T) {
	tests := map[string]string{
		"git-base-dir":   "GIT_BASE_DIR",
		"backup-enabled": "BACKUP_ENABLED",
		"debug":          "DEBUG",
		"":               "",
	}
	for in, want := range tests {
		if got := StringToEnvVar(in); got != want {
			t.Errorf("StringToEnvVar(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFileAndDirExists(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing")

	if !FileExists(file) {
		t.Error("FileExists(file) = false, want true")
	}
	if FileExists(dir) {
		t.Error("FileExists(dir) = true, want false")
	}
	if FileExists(missing) {
		t.Error("FileExists(missing) = true, want false")
	}

	if !DirExists(dir) {
		t.Error("DirExists(dir) = false, want true")
	}
	if DirExists(file) {
		t.Error("DirExists(file) = true, want false")
	}
	if DirExists(missing) {
		t.Error("DirExists(missing) = true, want false")
	}
}

type validationSample struct {
	Name     string `json:"name" validate:"required"`
	Strategy string `json:"strategy" validate:"oneof=copy stash"`
	Code     string `json:"code" validate:"len=3"`
	Email    string `json:"email" validate:"omitempty,email"`
	Count    int    `json:"count" validate:"min=1,max=10"`
	Source   string `json:"source_dir"`
	Target   string `json:"target_dir" validate:"nefield=Source"`
}

func TestCreateValidationErrorMessage(t *testing.T) {
	data := validationSample{
		Name:     "",
		Strategy: "rsync",
		Code:     "ab",
		Email:    "not-an-email",
		Count:    0,
		Source:   "same",
		Target:   "same",
	}

	err := validator.New().Struct(data)
	if err == nil {
		t.Fatal("expected validation errors")
	}

	msg := CreateValidationErrorMessage(err, data)

	wants := []string{
		"Name is required.",
		"Strategy must be one of [copy, stash].",
		"Code must have 3 characters.",
		"Email must be a valid email.",
		"Count must be greater than 1.",
		"Target must be different from source_dir.",
	}
	for _, want := range wants {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q does not contain %q", msg, want)
		}
	}
	if strings.HasSuffix(msg, " ") {
		t.Errorf("message should be trimmed, got %q", msg)
	}
}

func TestGetParamName(t *testing.T) {
	data := validationSample{}

	if got := GetParamName(data, "Source"); got != "source_dir" {
		t.Errorf("GetParamName(struct, Source) = %q, want %q", got, "source_dir")
	}
	if got := GetParamName(&data, "Source"); got != "source_dir" {
		t.Errorf("GetParamName(pointer, Source) = %q, want %q", got, "source_dir")
	}
	if got := GetParamName(data, "Unknown"); got != "Unknown" {
		t.Errorf("GetParamName(struct, Unknown) = %q, want %q", got, "Unknown")
	}
}

func TestCheckCommandsAvailableWithExistingCommands(t *testing.T) {
	// Only the success path is tested: a missing command calls Logger("fatal"),
	// which exits the process.
	CheckCommandsAvailable([]string{"sh", " "})
	CheckCommandsAvailable([]string{})
}

func TestCreateValidationErrorMessageOtherError(t *testing.T) {
	if got := CreateValidationErrorMessage(errors.New("plain error"), nil); got != "plain error" {
		t.Errorf("got %q", got)
	}
}

type allTagsSample struct {
	ID      string `validate:"uuid"`
	Number  string `validate:"number"`
	Bool    string `validate:"boolean"`
	Max     int    `validate:"max=1"`
	Text    any    `validate:"required,string"`
	Unknown string `validate:"hexcolor"`
}

func TestCreateValidationErrorMessageAllTags(t *testing.T) {
	validate := validator.New()
	_ = validate.RegisterValidation("string", func(fl validator.FieldLevel) bool { return false })
	data := allTagsSample{ID: "x", Number: "abc", Bool: "maybe", Max: 5, Text: 1, Unknown: "zz"}
	msg := CreateValidationErrorMessage(validate.Struct(data), data)
	for _, want := range []string{"ID must be a valid UUID.", "Number must be a number.", "Bool must be a boolean.",
		"Max must be less than 1.", "Text must be a string.", "Unknown is invalid."} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q does not contain %q", msg, want)
		}
	}
}

func TestFindExecutablePermissionError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	locked := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	if _, err := FindExecutable(filepath.Join(locked, "x")); err == nil {
		t.Error("stat error other than 'not exist' must be returned")
	}
}
