package desktop

import (
	"os"
	"path/filepath"
)

// Native integration tests override this at link time to isolate their WebView data.
var DataDirectory = "SpeedLimitFree"

func UserDataPath() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, DataDirectory, "WebView2"), nil
}
