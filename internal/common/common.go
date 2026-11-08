// Package common has common functions reusable
package common

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// FindExecutable checks if a file exists at the given path and is executable.
func FindExecutable(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil // File doesn't exist, not an error for the search logic itself
		}
		// Other error (e.g., permission denied to stat)
		return false, fmt.Errorf("[ERROR] Have permissions to execute? Check path '%s': %w", path, err)
	}

	// Check if it's a regular file and has execute permissions (for user, group, or others)
	// Mode()&0111 checks the execute bits.
	if !info.IsDir() && info.Mode()&0111 != 0 {
		return true, nil // Found and executable
	}

	// Exists but is a directory or not executable
	return false, nil
}

// CreateValidationErrorMessage based on validation tags.
// If the tag is not recognized, it will return a generic message.
func CreateValidationErrorMessage(err error, data any) string {
	var message string

	var validationErrors validator.ValidationErrors
	if !errors.As(err, &validationErrors) {
		return err.Error()
	}

	for _, err := range validationErrors {
		switch err.Tag() {
		case "required":
			message += fmt.Sprintf("%s is required. ", err.Field())
		case "len":
			message += fmt.Sprintf("%s must have %s characters. ", err.Field(), err.Param())
		case "oneof":
			message += fmt.Sprintf("%s must be one of [%s]. ", err.Field(), strings.Join(strings.Split(err.Param(), " "), ", "))
		case "email":
			message += fmt.Sprintf("%s must be a valid email. ", err.Field())
		case "min":
			message += fmt.Sprintf("%s must be greater than %s. ", err.Field(), err.Param())
		case "max":
			message += fmt.Sprintf("%s must be less than %s. ", err.Field(), err.Param())
		case "uuid":
			message += fmt.Sprintf("%s must be a valid UUID. ", err.Field())
		case "number":
			message += fmt.Sprintf("%s must be a number. ", err.Field())
		case "boolean":
			message += fmt.Sprintf("%s must be a boolean. ", err.Field())
		case "string":
			message += fmt.Sprintf("%s must be a string. ", err.Field())
		case "nefield":
			param := GetParamName(data, err.Param())
			message += fmt.Sprintf("%s must be different from %s. ", err.Field(), param)
		default:
			message += fmt.Sprintf("%s is invalid. ", err.Field())
		}
	}

	return strings.TrimSpace(message)
}

// GetParamName returns the name of the parameter based on the struct tag.
// If the tag is not found, it returns the parameter name.
func GetParamName(data any, param string) string {
	topStruct := reflect.TypeOf(data)

	if topStruct.Kind() == reflect.Pointer {
		topStruct = topStruct.Elem()
	}

	if field, ok := topStruct.FieldByName(param); ok {
		tag := field.Tag.Get("json")
		split := strings.SplitN(tag, ",", 2)
		if split[0] != "" {
			return split[0]
		}
	}
	return param
}

// StringToEnvVar transform strings to uppercase and substitute '-' by '_' if exists
func StringToEnvVar(s string) string {
	s = strings.ToUpper(s)
	s = strings.ReplaceAll(s, "-", "_")
	return s
}

// CheckCommandsAvailable verifies if all specified command-line tools are installed
// and accessible in the system's PATH.
// It returns a list of missing commands and an error if any are not found.
// If all commands are found, it returns nil, nil.
func CheckCommandsAvailable(commands []string) {
	missingCommands := []string{}

	if len(commands) == 0 {
		Logger("debug", "No commands specified for availability check.")
	}

	Logger("debug", "Checking availability of required commands: %v", commands)

	for _, cmdName := range commands {
		if strings.TrimSpace(cmdName) == "" {
			Logger("warning", "Empty command name provided in the list, skipping.")
			continue
		}
		// exec.LookPath searches for an executable named file in the directories
		// named by the PATH environment variable.
		// If file contains a slash, it is tried directly and the PATH is not consulted.
		// The result may be an absolute path or a path relative to the current directory.
		_, findErr := exec.LookPath(cmdName)
		if findErr != nil {
			// Error typically means the command was not found in PATH.
			// It could also be a permission issue for directories in PATH, but "not found" is most common.
			Logger("warning", "Command '%s' not found in system PATH: %v", cmdName, findErr)
			missingCommands = append(missingCommands, cmdName)
		} else {
			Logger("debug", "Command '%s' found in system PATH.", cmdName)
		}
	}

	if len(missingCommands) > 0 {
		Logger("fatal", "the following required command(s) were not found in your system PATH: %s. Please install them and ensure they are accessible.", strings.Join(missingCommands, ", "))
	}

	Logger("debug", "All specified commands (%v) are available in system PATH.", commands)
}

// FileExists checks if a file exists and is not a directory.
func FileExists(path string) bool {
	info, errStat := os.Stat(path)
	return errStat == nil && !info.IsDir()
}

// DirExists checks if a directory exists and is a directory.
func DirExists(path string) bool {
	info, errStat := os.Stat(path)
	return errStat == nil && info.IsDir()
}
