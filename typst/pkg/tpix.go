package pkg

import (
	"io"

	"github.com/typstify/tpix-cli/api"
	"looz.ws/typstify/service/settings"
)

type tpixCliReporter struct {
	w io.Writer
}

func (r tpixCliReporter) Report(message string) {
	if r.w != nil {
		r.w.Write([]byte(message))
	}
}

var _ api.ApiKeyProvider = (*tpixApiKeyProvider)(nil)

type tpixApiKeyProvider struct {
	setting *settings.TpixSettings
}

func (p *tpixApiKeyProvider) Get() string {
	return p.setting.ApiKey
}
