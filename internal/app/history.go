package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const queryHistoryLimit = 500
const maxQueryHistoryFileSize = 32 << 20

func queryHistoryPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "query-history.jsonl")
}

func loadQueryHistory(path string) ([]string, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("query history is not a regular file")
	}
	if st.Size() > maxQueryHistoryFileSize {
		return nil, fmt.Errorf("query history exceeds %d bytes", maxQueryHistoryFileSize)
	}
	if err := f.Chmod(0600); err != nil {
		return nil, err
	}

	var history []string
	scanner := bufio.NewScanner(io.LimitReader(f, maxQueryHistoryFileSize))
	scanner.Buffer(make([]byte, 4096), 128<<10)
	for scanner.Scan() {
		var query string
		if err := json.Unmarshal(scanner.Bytes(), &query); err != nil {
			return nil, fmt.Errorf("invalid query history entry: %w", err)
		}
		history = append(history, query)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(history) > queryHistoryLimit {
		history = history[len(history)-queryHistoryLimit:]
	}
	return history, nil
}

// saveQueryHistory serializes read-modify-write operations across composer
// processes, then replaces the JSONL file atomically while holding the lock.
func saveQueryHistory(path, query string) ([]string, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	if err := lock.Chmod(0600); err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return nil, err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	history, err := loadQueryHistory(path)
	if err != nil {
		return nil, err
	}
	history = append(history, query)
	if len(history) > queryHistoryLimit {
		history = history[len(history)-queryHistoryLimit:]
	}

	tmp, err := os.CreateTemp(dir, ".query-history-*")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return nil, err
	}
	writer := bufio.NewWriter(tmp)
	for _, entry := range history {
		b, err := json.Marshal(entry)
		if err != nil {
			tmp.Close()
			return nil, err
		}
		if _, err := writer.Write(append(b, '\n')); err != nil {
			tmp.Close()
			return nil, err
		}
	}
	if err := writer.Flush(); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return nil, err
	}
	if dirHandle, err := os.Open(dir); err == nil {
		err = dirHandle.Sync()
		_ = dirHandle.Close()
		if err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}
	return history, nil
}

func formatHistoryError(err error) string {
	if err == nil {
		return ""
	}
	return strings.ReplaceAll(err.Error(), "\n", " ")
}
