package desktop

import (
	"github.com/wailsapp/wails/v2/pkg/options"
	"golang.org/x/sys/windows/registry"
)

// BackgroundColour matches the initial WebView canvas to the Windows app theme.
// The native frame and CSS media query handle subsequent preference changes.
func BackgroundColour() *options.RGBA {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err == nil {
		defer key.Close()
		if light, _, err := key.GetIntegerValue("AppsUseLightTheme"); err == nil && light == 0 {
			return &options.RGBA{R: 32, G: 35, B: 39, A: 255}
		}
	}
	return &options.RGBA{R: 255, G: 255, B: 255, A: 255}
}
