package cmds

import (
	tea "charm.land/bubbletea/v2"
	"github.com/filipemolina/chore-crusher/src/appstyles"
	"github.com/filipemolina/chore-crusher/src/config"
)

// ApplyThemeMsg asks AppModel to make a theme permanent: set it as the
// active theme and write it to the config file. The modal has already
// previewed it live; this is the "user pressed Enter" step.
type ApplyThemeMsg struct {
	Name string
}

// ApplyTheme sets the named theme as active and persists it. Returns the
// message for AppModel to handle; any config-write error is reported
// through ThemeAppliedMsg so the app can surface it.
func ApplyTheme(name string) tea.Cmd {
	return func() tea.Msg {
		if !appstyles.SetTheme(name) {
			return ThemeAppliedMsg{Name: name, Err: errUnknownTheme(name)}
		}

		cfg, _ := config.LoadConfig()
		cfg.Theme = name
		if err := config.SaveConfig(cfg); err != nil {
			return ThemeAppliedMsg{Name: name, Err: err}
		}

		return ThemeAppliedMsg{Name: name}
	}
}

// ThemeAppliedMsg is the result of an ApplyTheme command. AppModel reads Err
// to decide whether to report a persist failure; the modal closes either way
// (the theme was already previewed, so the user saw their choice).
type ThemeAppliedMsg struct {
	Name string
	Err  error
}

type errUnknownTheme string

func (e errUnknownTheme) Error() string {
	return "unknown theme: " + string(e)
}
