package service

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nuntiiscore/envmerge/internal/envmerge/field"
)

type Service struct {
	force bool
	src   map[string]string
	dst   *field.File
}

func New(src, dst string, force bool) (*Service, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("cannot determine caller dir: %w", err)
	}

	srcContent, err := readSrcFile(dir, src)
	if err != nil {
		return nil, fmt.Errorf("error reading source file: %w", err)
	}

	dstFile, err := readDstFile(dir, dst)
	if err != nil {
		return nil, fmt.Errorf("error reading destination file: %w", err)
	}

	return &Service{
		force: force,
		dst:   dstFile,
		src:   srcContent,
	}, nil
}

func (s *Service) Run() error {
	defer func() {
		if s.dst != nil && s.dst.Dsc != nil {
			_ = s.dst.Dsc.Close()
		}
	}()

	if s.force {
		updates := s.determineUpdates()
		if len(updates) > 0 {
			if err := s.writeVars(updates, true); err != nil {
				return fmt.Errorf("error writing vars (force): %w", err)
			}
		}
	} else {
		newVars := s.determineNewVars()
		if len(newVars) > 0 {
			if err := s.writeVars(newVars, false); err != nil {
				return fmt.Errorf("error writing new vars: %w", err)
			}
		}
	}

	slog.Default().Info("dotenv synced")
	return nil
}

func (s *Service) determineNewVars() map[string]string {
	newVars := make(map[string]string, len(s.src))
	for variable, val := range s.src {
		if _, ok := s.dst.Data[variable]; !ok {
			newVars[variable] = val
		}
	}

	return newVars
}

func (s *Service) determineUpdates() map[string]string {
	updates := make(map[string]string, len(s.src))
	for k, v := range s.src {
		old, ok := s.dst.Data[k]
		if !ok || old != v {
			updates[k] = v
		}
	}

	return updates
}

func (s *Service) writeVars(vars map[string]string, isForce bool) error {
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	header := "\n# envmerge sync run: %s\n"
	if isForce {
		header = "\n# envmerge sync run (force): %s\n"
	}

	if _, err := s.dst.Dsc.WriteString(fmt.Sprintf(header, time.Now().Format(time.DateTime))); err != nil {
		return fmt.Errorf("error writing header: %w", err)
	}

	for _, k := range keys {
		v := vars[k]

		line := fmt.Sprintf("%s=%s\n", k, formatEnvValue(v))
		if _, err := s.dst.Dsc.WriteString(line); err != nil {
			return fmt.Errorf("error writing var %q: %w", k, err)
		}
	}

	return nil
}

func formatEnvValue(v string) string {
	needsQuotes := false
	for _, ch := range v {
		switch ch {
		case ' ', '\t', '\n', '\r', '#', '"':
			needsQuotes = true
		}
		if needsQuotes {
			break
		}
	}

	if !needsQuotes {
		return v
	}

	escaped := strings.ReplaceAll(v, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)

	return `"` + escaped + `"`
}

func readSrcFile(dir, file string) (map[string]string, error) {
	filePath := resolvePath(dir, file)
	slog.Default().Info("Reading file", "path", filePath)

	content, err := os.Open(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, field.ErrFileDoesNotExist
		}
		return nil, fmt.Errorf("open %q: %w", filePath, err)
	}
	defer content.Close()

	data, err := fileContent(content)
	if err != nil {
		return nil, fmt.Errorf("error reading file %q: %w", filePath, err)
	}

	return data, nil
}

func readDstFile(dir, file string) (*field.File, error) {
	filePath := resolvePath(dir, file)
	slog.Default().Info("Reading file", "path", filePath)

	content, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", filePath, err)
	}

	if _, err := content.Seek(0, io.SeekStart); err != nil {
		_ = content.Close()
		return nil, fmt.Errorf("seek start %q: %w", filePath, err)
	}

	data, err := fileContent(content)
	if err != nil {
		_ = content.Close()
		return nil, fmt.Errorf("error reading file %q: %w", filePath, err)
	}

	if _, err := content.Seek(0, io.SeekEnd); err != nil {
		_ = content.Close()
		return nil, fmt.Errorf("seek end %q: %w", filePath, err)
	}

	return &field.File{
		Dsc:  content,
		Data: data,
	}, nil
}

func resolvePath(dir, file string) string {
	if filepath.IsAbs(file) {
		return file
	}

	return filepath.Join(dir, file)
}

func fileContent(r io.Reader) (map[string]string, error) {
	scanner := bufio.NewScanner(r)
	const maxToken = 1024 * 1024 // 1MB
	scanner.Buffer(make([]byte, 1024), maxToken)

	env := make(map[string]string)

	var (
		currentKey   string
		currentValue strings.Builder
		inMultiline  bool
	)

	for scanner.Scan() {
		rawLine := scanner.Text()

		if inMultiline {
			line := strings.TrimSuffix(rawLine, "\r")

			trimmedRight := strings.TrimRight(line, " \t")
			if strings.HasSuffix(trimmedRight, `"`) && !strings.HasSuffix(trimmedRight, `\"`) {
				trimmedRight = strings.TrimSuffix(trimmedRight, `"`)
				currentValue.WriteString("\n" + trimmedRight)
				env[currentKey] = currentValue.String()

				inMultiline = false
				currentKey = ""
				currentValue.Reset()
			} else {
				currentValue.WriteString("\n" + line)
			}

			continue
		}

		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid env line: %q", line)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if strings.HasPrefix(value, `"`) && !strings.HasSuffix(value, `"`) {
			inMultiline = true
			currentKey = key
			currentValue.WriteString(strings.TrimPrefix(value, `"`))

			continue
		}

		env[key] = strings.Trim(value, `"`)
	}

	if inMultiline {
		return nil, fmt.Errorf("unterminated multiline value for key %q", currentKey)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning file: %w", err)
	}

	return env, nil
}
