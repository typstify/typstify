package service

import (
	"io"
	"log"
	"sync"
	"sync/atomic"
	"time"

	cli "github.com/typstify/tpix-cli"
	tpix "github.com/typstify/tpix-cli"
	"github.com/typstify/tpix-cli/config"
	"looz.ws/typstify/service/settings"
)

type tpixCredentialProvider struct {
	setting *settings.TpixSettings
}

func (c *tpixCredentialProvider) Load() (config.Credentials, error) {
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

const (
	profileUpdateInterval = time.Minute * 3
)

type TpixSession struct {
	Username    string
	Email       string
	LastLoginAt string
	Subscribed  bool
}

type TpixSessionService struct {
	setting        *settings.TpixSettings
	session        atomic.Pointer[TpixSession]
	lastUpdateTime time.Time
	updating       atomic.Bool
	mu             sync.Mutex
}

func (t *TpixSessionService) Authenticated() bool {
	return t.setting.LoginAt > 0
}

// Session returns the active TPIX session of the user.
func (t *TpixSessionService) Session() *TpixSession {
	if !t.Authenticated() {
		return nil
	}

	sn := t.session.Load()

	t.mu.Lock()
	lastUpdateTime := t.lastUpdateTime
	t.mu.Unlock()
	if sn == nil || time.Since(lastUpdateTime) > profileUpdateInterval {
		go t.updateSession()
	}

	if sn == nil {
		// return a temporal session before update finish.
		return &TpixSession{
			Username:    t.setting.Username,
			Email:       t.setting.Email,
			LastLoginAt: time.UnixMilli(t.setting.LoginAt).Format(time.DateTime),
		}
	}

	activeSn := *sn
	return &activeSn
}

// Login authenticates the user against TPIX server via the Device Flow of TPIX.
// When user is already logged in, it only update user profile.
func (t *TpixSessionService) Login() error {
	if t.setting.LoginAt <= 0 {
		if err := t.login(); err != nil {
			return err
		}
	}

	return t.updateSession()
}

func (t *TpixSessionService) Logout() {
	t.setting.Clear()
	t.session.Store(nil)
}

func (t *TpixSessionService) login() error {
	resp, err := tpix.StartLogin()
	if err != nil {
		return err
	}
	tokenResp, err := cli.PollLoginResult(resp.DeviceCode, resp.ExpiresIn, nil)
	if err != nil {
		log.Printf("Login failed: %v", err)
		return err
	}

	t.setting.AccessToken = tokenResp.AccessToken
	t.setting.RefreshToken = tokenResp.RefreshToken
	t.setting.LoginAt = time.Now().UnixMilli()
	return t.setting.Save()
}

func (t *TpixSessionService) updateSession() error {
	if !t.updating.CompareAndSwap(false, true) {
		return nil
	}
	defer t.updating.Store(false)

	profile, err := cli.GetUserProfile()
	if err != nil {
		log.Printf("update session error: %s", err)
		return err
	}

	loginAt := time.UnixMilli(t.setting.LoginAt)

	session := TpixSession{
		Username:    profile.Username,
		Email:       profile.Email,
		LastLoginAt: loginAt.Format(time.DateTime),
		Subscribed:  profile.Subscribed,
	}

	t.setting.Username = session.Username
	t.setting.Email = session.Email
	t.setting.Save()

	t.session.Store(&session)
	t.mu.Lock()
	t.lastUpdateTime = time.Now()
	t.mu.Unlock()

	return nil
}
