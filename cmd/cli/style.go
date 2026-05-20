package main

import "charm.land/lipgloss/v2"

// ── shared colour palette ─────────────────────────────────────────────────────

var (
	clrPurple = lipgloss.Color("#7C3AED")
	clrGreen  = lipgloss.Color("#10B981")
	clrRed    = lipgloss.Color("#EF4444")
	clrGray   = lipgloss.Color("#6B7280")
	clrWhite  = lipgloss.Color("#E5E7EB")
	clrAmber  = lipgloss.Color("#F59E0B")
)

// ── semantic styles ───────────────────────────────────────────────────────────

var (
	styleOK      = lipgloss.NewStyle().Foreground(clrGreen)
	styleActive  = lipgloss.NewStyle().Bold(true).Foreground(clrGreen)
	styleErr     = lipgloss.NewStyle().Foreground(clrRed)
	styleWarn    = lipgloss.NewStyle().Foreground(clrAmber)
	styleKey     = lipgloss.NewStyle().Bold(true).Foreground(clrPurple)
	styleSection = lipgloss.NewStyle().Bold(true).Foreground(clrGray)
	styleDim     = lipgloss.NewStyle().Foreground(clrGray)
	styleColHdr  = lipgloss.NewStyle().Bold(true).Foreground(clrGray)
	styleBold    = lipgloss.NewStyle().Bold(true)
	styleCurrent = lipgloss.NewStyle().Foreground(clrWhite)
	styleSelected = lipgloss.NewStyle().Bold(true).Foreground(clrWhite)
)
