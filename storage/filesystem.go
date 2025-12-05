package storage

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/nabetse28/golang-mail-billing/logging"
)

// ExpandHome expands a path starting with "~" to the user's home directory.
func ExpandHome(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("empty path")
	}

	if !strings.HasPrefix(path, "~") {
		return path, nil
	}

	usr, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("failed to get current user: %w", err)
	}

	return filepath.Join(usr.HomeDir, strings.TrimPrefix(path, "~")), nil
}

func spanishMonthName(month int) string {
	switch time.Month(month) {
	case time.January:
		return "Enero"
	case time.February:
		return "Febrero"
	case time.March:
		return "Marzo"
	case time.April:
		return "Abril"
	case time.May:
		return "Mayo"
	case time.June:
		return "Junio"
	case time.July:
		return "Julio"
	case time.August:
		return "Agosto"
	case time.September:
		return "Septiembre"
	case time.October:
		return "Octubre"
	case time.November:
		return "Noviembre"
	case time.December:
		return "Diciembre"
	default:
		// Fallback to English name if something weird happens
		return time.Month(month).String()
	}
}

// EnsureInvoiceDir ensures the directory Base/{Year}/{MonthNameEs} exists and returns its full path.
// Example: ~/Documents/Personal/Facturas/2025/Noviembre
func EnsureInvoiceDir(basePath string, year int, month int) (string, error) {
	expandedBase, err := ExpandHome(basePath)
	if err != nil {
		return "", err
	}

	monthName := spanishMonthName(month)

	dir := filepath.Join(
		expandedBase,
		fmt.Sprintf("%04d", year),
		monthName,
	)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	logging.Infof("Ensured directory exists: %s", dir)
	return dir, nil
}

// WriteFileUnique writes data to dir/filename, adding a suffix if the file already exists.
func WriteFileUnique(dir, filename string, data []byte) (string, error) {
	if filename == "" {
		filename = "attachment"
	}

	targetPath := filepath.Join(dir, filename)

	// If file exists, append a numeric suffix before the extension.
	if _, err := os.Stat(targetPath); err == nil {
		ext := filepath.Ext(filename)
		name := strings.TrimSuffix(filename, ext)

		for i := 1; ; i++ {
			newName := fmt.Sprintf("%s_%d%s", name, i, ext)
			targetPath = filepath.Join(dir, newName)
			if _, err := os.Stat(targetPath); os.IsNotExist(err) {
				break
			}
		}
	}

	if err := os.WriteFile(targetPath, data, 0o644); err != nil {
		return "", fmt.Errorf("failed to write file %s: %w", targetPath, err)
	}

	logging.Infof("Saved attachment to %s", targetPath)
	return targetPath, nil
}
