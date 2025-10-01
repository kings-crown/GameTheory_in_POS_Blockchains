package godotenv

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Load reads environment variables from one or more files and applies them to the
// current process. It supports the simple KEY=value format used by the original library.
// Lines beginning with '#' are ignored, and surrounding quotes are stripped when present.
func Load(filenames ...string) error {
	if len(filenames) == 0 {
		filenames = []string{".env"}
	}

	var loadErr error

	for _, name := range filenames {
		if err := loadFile(name); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if loadErr == nil {
					loadErr = err
				}
				continue
			}
			return err
		}
		loadErr = nil
	}

	return loadErr
}

func loadFile(path string) error {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 || line[0] == '#' {
			continue
		}

		key, value, ok := parseLine(line)
		if !ok {
			continue
		}

		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set env %s: %w", key, err)
		}
	}

	return scanner.Err()
}

func parseLine(line string) (string, string, bool) {
	var (
		key   string
		value string
	)

	for i := 0; i < len(line); i++ {
		if line[i] == '=' {
			key = line[:i]
			value = line[i+1:]
			break
		}
	}

	if key == "" {
		return "", "", false
	}

	value = trimWhitespace(value)
	value = trimQuotes(value)
	key = trimWhitespace(key)

	if key == "" {
		return "", "", false
	}

	return key, value, true
}

func trimWhitespace(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r' || s[i] == '\n') {
		i++
	}

	j := len(s)
	for j > i && (s[j-1] == ' ' || s[j-1] == '\t' || s[j-1] == '\r' || s[j-1] == '\n') {
		j--
	}

	return s[i:j]
}

func trimQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}

	return s
}
