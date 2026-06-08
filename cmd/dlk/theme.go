package main

import (
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// dlkPromptTheme is the monochrome theme applied to every interactive huh prompt.
// Focused element = bold white, secondary text = gray, errors = red. No accent
// colours — keeps the CLI prompts calm and professional instead of huh's default
// pink/magenta. Colours come from the shared palette in style.go.
var dlkPromptTheme huh.Theme = huh.ThemeFunc(func(isDark bool) *huh.Styles {
	t := huh.ThemeBase(isDark)

	f := &t.Focused
	f.Base = f.Base.BorderForeground(clrGray)
	f.Card = f.Base
	f.Title = f.Title.Bold(true).Foreground(clrWhite)
	f.NoteTitle = f.NoteTitle.Bold(true).Foreground(clrWhite)
	f.Description = f.Description.Foreground(clrGray)
	f.ErrorIndicator = f.ErrorIndicator.Foreground(clrRed)
	f.ErrorMessage = f.ErrorMessage.Foreground(clrRed)
	f.SelectSelector = f.SelectSelector.Bold(true).Foreground(clrWhite)
	f.MultiSelectSelector = f.MultiSelectSelector.Bold(true).Foreground(clrWhite)
	f.Option = f.Option.Foreground(clrGray)
	f.SelectedOption = f.SelectedOption.Bold(true).Foreground(clrWhite)
	f.SelectedPrefix = f.SelectedPrefix.Foreground(clrWhite)
	f.UnselectedOption = f.UnselectedOption.Foreground(clrGray)
	f.UnselectedPrefix = f.UnselectedPrefix.Foreground(clrGray)
	f.NextIndicator = f.NextIndicator.Foreground(clrWhite)
	f.PrevIndicator = f.PrevIndicator.Foreground(clrWhite)
	f.FocusedButton = f.FocusedButton.Foreground(lipgloss.Color("0")).Background(clrWhite).Bold(true)
	f.BlurredButton = f.BlurredButton.Foreground(clrGray).Background(lipgloss.Color("0"))
	f.TextInput.Cursor = f.TextInput.Cursor.Foreground(clrWhite)
	f.TextInput.Prompt = f.TextInput.Prompt.Foreground(clrGray)
	f.TextInput.Placeholder = f.TextInput.Placeholder.Foreground(clrGray)
	f.TextInput.Text = f.TextInput.Text.Foreground(clrWhite)

	t.Blurred = t.Focused
	t.Blurred.Base = t.Blurred.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Blurred.Card = t.Blurred.Base
	t.Blurred.Title = t.Blurred.Title.Foreground(clrGray)
	t.Blurred.NextIndicator = lipgloss.NewStyle()
	t.Blurred.PrevIndicator = lipgloss.NewStyle()

	t.Group.Title = t.Focused.Title
	t.Group.Description = t.Focused.Description
	return t
})
