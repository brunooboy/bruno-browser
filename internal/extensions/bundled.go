package extensions

import (
	"os"
	"path/filepath"
)

const (
	BrunoINSSISTFileName = "Bruno-INSSIST.crx"
	BrunoINSSISTSHA256   = "1199419b25f78202d0c3cb8828ffcf3cdbb92c8e9c7059b6043f4078ee9070ad"
)

// FindBrunoINSSIST locates the packaged CRX beside an installed/portable app
// or in the repository asset directory during development.
func FindBrunoINSSIST() (string, bool) {
	candidates := make([]string, 0, 4)
	if executable, err := os.Executable(); err == nil {
		directory := filepath.Dir(executable)
		candidates = append(candidates,
			filepath.Join(directory, "native-extensions", BrunoINSSISTFileName),
			filepath.Join(directory, "assets", "native-extensions", BrunoINSSISTFileName),
		)
	}
	if workingDirectory, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(workingDirectory, "assets", "native-extensions", BrunoINSSISTFileName),
			filepath.Join(workingDirectory, "..", "assets", "native-extensions", BrunoINSSISTFileName),
		)
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && info.Mode().IsRegular() {
			absolute, absoluteErr := filepath.Abs(candidate)
			return absolute, absoluteErr == nil
		}
	}
	return "", false
}
