package service

import (
	"io"

	"github.com/typstify/tpix-cli/config"
	"looz.ws/typstify/service/settings"
)

type tpixCredentialProvider struct {
	setting *settings.TpixSettings
}

func (c *tpixCredentialProvider) Load() (config.Credentials, error) {
	err := c.setting.Load()
	if err != nil {
		return config.Credentials{}, err
	}

	return config.Credentials{
		AccessToken:  c.setting.AccessToken,
		RefreshToken: c.setting.RefreshToken,
	}, nil
}

func (c *tpixCredentialProvider) Save(cred config.Credentials) error {
	c.setting.AccessToken = cred.AccessToken
	c.setting.RefreshToken = cred.RefreshToken

	return c.setting.Save()
}

type tpixCliReporter struct {
	w io.Writer
}

func (r tpixCliReporter) Report(message string) {
	if r.w != nil {
		r.w.Write([]byte(message))
	}
}
