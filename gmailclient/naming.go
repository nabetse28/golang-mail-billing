package gmailclient

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func RenameDownloadedAttachments(dir string, company string, msgDate time.Time, runTS string, atts []DownloadedAttachment) error {
	company = sanitizeCompany(company)
	date := msgDate.Format("20060102")

	byExt := map[string][]DownloadedAttachment{}
	for _, a := range atts {
		ext := strings.ToLower(strings.TrimPrefix(a.Ext, "."))
		byExt[ext] = append(byExt[ext], a)
	}

	// Orden estable por nombre original
	for ext := range byExt {
		sort.SliceStable(byExt[ext], func(i, j int) bool {
			return byExt[ext][i].OriginalFilename < byExt[ext][j].OriginalFilename
		})
	}

	orderedExt := []string{"pdf", "xml"}
	seen := map[string]bool{"pdf": true, "xml": true}
	for ext := range byExt {
		if !seen[ext] {
			orderedExt = append(orderedExt, ext)
		}
	}

	for _, ext := range orderedExt {
		list := byExt[ext]
		if len(list) == 0 {
			continue
		}

		for i, a := range list {
			idx := i + 1
			base := fmt.Sprintf("%s_%d_%s_%s", company, idx, date, runTS)
			newName := fmt.Sprintf("%s.%s", base, ext)
			newPath := filepath.Join(dir, newName)

			newPath = avoidCollision(newPath)

			if err := os.Rename(a.LocalPath, newPath); err != nil {
				return fmt.Errorf("rename %s -> %s: %w", a.LocalPath, newPath, err)
			}
		}
	}

	return nil
}

func avoidCollision(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for n := 2; n < 999; n++ {
		p := fmt.Sprintf("%s_%d%s", base, n, ext)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return p
		}
	}
	return path
}

func avoidCollisionWithTimestamp(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}

	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)

	ts := time.Now().Format("20060102T150405")

	newPath := fmt.Sprintf("%s_%s%s", base, ts, ext)
	return newPath
}
