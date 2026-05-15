package main

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"downlink/pkg/downlinkclient"
)

// ── messages ─────────────────────────────────────────────────────────────────

type queuePollMsg struct{}
type queueDataMsg downlinkclient.QueueStatus
type queueActionMsg string

// ── styles ───────────────────────────────────────────────────────────────────

var (
	qmPurple = lipgloss.Color("#7C3AED")
	qmGreen  = lipgloss.Color("#10B981")
	qmRed    = lipgloss.Color("#EF4444")
	qmGray   = lipgloss.Color("#6B7280")
	qmWhite  = lipgloss.Color("#E5E7EB")

	qmTitle     = lipgloss.NewStyle().Bold(true)
	qmActive    = lipgloss.NewStyle().Bold(true).Foreground(qmGreen)
	qmDim       = lipgloss.NewStyle().Foreground(qmGray)
	qmCurrent   = lipgloss.NewStyle().Foreground(qmWhite)
	qmKey       = lipgloss.NewStyle().Bold(true).Foreground(qmPurple)
	qmOk        = lipgloss.NewStyle().Foreground(qmGreen)
	qmErrStyle  = lipgloss.NewStyle().Foreground(qmRed)
	qmColHeader = lipgloss.NewStyle().Bold(true).Foreground(qmGray)
)

// ── model ─────────────────────────────────────────────────────────────────────

type queueMonitorModel struct {
	client    *downlinkclient.DownlinkClient
	status    downlinkclient.QueueStatus
	spin      spinner.Model
	prog      progress.Model
	width     int
	height    int
	maxSeen   int
	actionMsg string
	actionErr bool
}

func newQueueMonitorModel(client *downlinkclient.DownlinkClient) queueMonitorModel {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	pr := progress.New(
		progress.WithDefaultBlend(),
		progress.WithoutPercentage(),
		progress.WithWidth(40),
	)
	return queueMonitorModel{client: client, spin: sp, prog: pr}
}

func (m queueMonitorModel) Init() tea.Cmd {
	return tea.Batch(
		tea.Cmd(m.spin.Tick),
		cmdFetchStatus(m.client),
		cmdPollTick(),
	)
}

func cmdFetchStatus(c *downlinkclient.DownlinkClient) tea.Cmd {
	return func() tea.Msg {
		return queueDataMsg(c.GetQueueStatus())
	}
}

func cmdPollTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return queuePollMsg{}
	})
}

func cmdQueueAction(c *downlinkclient.DownlinkClient, action string) tea.Cmd {
	return func() tea.Msg {
		var err error
		switch action {
		case "start":
			err = c.StartQueue()
		case "stop":
			err = c.StopQueue()
		case "clear":
			err = c.ClearQueue()
		}
		if err != nil {
			return queueActionMsg("error: " + err.Error())
		}
		return queueActionMsg("✓ " + action)
	}
}

func (m queueMonitorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		barW := max(msg.Width-30, 10)
		m.prog.SetWidth(barW)
		return m, nil

	case queueDataMsg:
		m.status = downlinkclient.QueueStatus(msg)
		total := len(m.status.Queue)
		if m.status.IsProcessing {
			total++
		}
		if total > m.maxSeen {
			m.maxSeen = total
		}
		return m, nil

	case queuePollMsg:
		return m, tea.Batch(cmdFetchStatus(m.client), cmdPollTick())

	case queueActionMsg:
		s := string(msg)
		m.actionErr = strings.HasPrefix(s, "error:")
		m.actionMsg = s
		return m, cmdFetchStatus(m.client)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "s":
			m.actionMsg = ""
			return m, cmdQueueAction(m.client, "start")
		case "x":
			m.actionMsg = ""
			return m, cmdQueueAction(m.client, "stop")
		case "c":
			m.actionMsg = ""
			return m, cmdQueueAction(m.client, "clear")
		}
	}
	return m, nil
}

func (m queueMonitorModel) View() tea.View {
	if m.width == 0 {
		return tea.NewView("\n  Loading…\n")
	}

	var b strings.Builder
	w := m.width

	// header
	b.WriteString("\n")
	titleStr := qmTitle.Render("Analysis Queue")
	var stateStr string
	if m.status.IsProcessing {
		stateStr = qmActive.Render("● PROCESSING")
	} else {
		stateStr = qmDim.Render("○  IDLE")
	}
	gap := max(w-lipgloss.Width(titleStr)-lipgloss.Width(stateStr)-4, 1)
	b.WriteString("  " + titleStr + strings.Repeat(" ", gap) + stateStr + "\n")
	b.WriteString("  " + qmDim.Render(strings.Repeat("─", w-4)) + "\n\n")

	// current article
	if m.status.IsProcessing {
		cur := m.status.CurrentTitle
		if cur == "" {
			cur = "processing…"
		}
		maxW := max(w-14, 10)
		b.WriteString("  " + qmDim.Render("now") + "   " + m.spin.View() + " " + qmCurrent.Render(truncate(cur, maxW)) + "\n\n")
	} else {
		b.WriteString("  " + qmDim.Render("now") + "   " + qmDim.Render("—") + "\n\n")
	}

	// progress bar
	pending := len(m.status.Queue)
	active := pending
	if m.status.IsProcessing {
		active++
	}
	var pct float64
	if m.maxSeen > 0 {
		pct = 1.0 - float64(active)/float64(m.maxSeen)
	}
	var countStr string
	if m.maxSeen > 0 {
		countStr = fmt.Sprintf("%d / %d", m.maxSeen-active, m.maxSeen)
	} else {
		countStr = fmt.Sprintf("%d queued", pending)
	}
	b.WriteString("  " + m.prog.ViewAs(pct) + "  " + qmDim.Render(countStr) + "\n\n")

	// queue table
	if len(m.status.Queue) > 0 {
		titleColW := max(w-52, 20)
		hFmt := fmt.Sprintf("  %%3s  %%-%ds  %%-14s  %%-20s", titleColW)
		rFmt := fmt.Sprintf("  %%3d  %%-%ds  %%-14s  %%-20s", titleColW)

		b.WriteString(qmColHeader.Render(fmt.Sprintf(hFmt, "#", "TITLE", "PROFILE", "MODEL")) + "\n")
		b.WriteString(qmDim.Render(fmt.Sprintf(hFmt,
			strings.Repeat("─", 3),
			strings.Repeat("─", titleColW),
			strings.Repeat("─", 14),
			strings.Repeat("─", 20),
		)) + "\n")

		maxRows := max(m.height-16, 3)
		shown := min(len(m.status.Queue), maxRows)
		for i := range shown {
			j := m.status.Queue[i]
			profile := j.ProviderName
			if profile == "" {
				profile = j.ProviderType
			}
			if profile == "" {
				profile = "—"
			}
			model := j.ModelName
			if model == "" {
				model = "—"
			}
			b.WriteString(fmt.Sprintf(rFmt,
				i+1,
				truncate(j.ArticleTitle, titleColW),
				truncate(profile, 14),
				truncate(model, 20),
			) + "\n")
		}
		if len(m.status.Queue) > maxRows {
			b.WriteString(qmDim.Render(fmt.Sprintf("  … %d more", len(m.status.Queue)-maxRows)) + "\n")
		}
		b.WriteString("\n")
	} else if !m.status.IsProcessing {
		b.WriteString("  " + qmDim.Render("Queue is empty") + "\n\n")
	}

	// action feedback
	if m.actionMsg != "" {
		if m.actionErr {
			b.WriteString("  " + qmErrStyle.Render(m.actionMsg) + "\n\n")
		} else {
			b.WriteString("  " + qmOk.Render(m.actionMsg) + "\n\n")
		}
	}

	// help
	help := qmKey.Render("s") + qmDim.Render(" start") + "   " +
		qmKey.Render("x") + qmDim.Render(" stop") + "   " +
		qmKey.Render("c") + qmDim.Render(" clear") + "   " +
		qmKey.Render("q") + qmDim.Render(" quit")
	b.WriteString("  " + help + "\n")

	return tea.NewView(b.String())
}

func runQueueMonitor(client *downlinkclient.DownlinkClient) error {
	m := newQueueMonitorModel(client)
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}
