package storage

import "os"

func FileExists(path string, filename string) bool {
	_, err := os.Stat(path + "/" + filename)
	return err == nil
}
