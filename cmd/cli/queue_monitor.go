package main

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"downlink/pkg/downlinkclient"
)

// ── messages ─────────────────────────────────────────────────────────────────

type queuePollMsg         struct{}
type queueDataMsg         downlinkclient.QueueStatus
type queueStreamUpdateMsg downlinkclient.QueueStatus
type queueProgressMsg     downlinkclient.AnalysisProgress
type queueStreamErrMsg    struct{ err error }
type queueActionMsg       string

// ── task step tracking ────────────────────────────────────────────────────────

type taskStatus int

const (
	taskPending   taskStatus = iota
	taskActive               // "started" received
	taskCompleted            // "completed" received
	taskError                // "error" received
)

type taskStep struct {
	name   string
	status taskStatus
}

// ── styles ───────────────────────────────────────────────────────────────────

var (
	qmPurple = lipgloss.Color("#7C3AED")
	qmGreen  = lipgloss.Color("#10B981")
	qmRed    = lipgloss.Color("#EF4444")
	qmGray   = lipgloss.Color("#6B7280")
	qmWhite  = lipgloss.Color("#E5E7EB")
	qmAmber  = lipgloss.Color("#F59E0B")

	qmTitle     = lipgloss.NewStyle().Bold(true)
	qmActive    = lipgloss.NewStyle().Bold(true).Foreground(qmGreen)
	qmDim       = lipgloss.NewStyle().Foreground(qmGray)
	qmCurrent   = lipgloss.NewStyle().Foreground(qmWhite)
	qmSelected  = lipgloss.NewStyle().Bold(true).Foreground(qmWhite)
	qmKey       = lipgloss.NewStyle().Bold(true).Foreground(qmPurple)
	qmOk        = lipgloss.NewStyle().Foreground(qmGreen)
	qmErrStyle  = lipgloss.NewStyle().Foreground(qmRed)
	qmWarn      = lipgloss.NewStyle().Foreground(qmAmber)
	qmColHeader = lipgloss.NewStyle().Bold(true).Foreground(qmGray)
	qmStepOk    = lipgloss.NewStyle().Foreground(qmGreen)
	qmStepErr   = lipgloss.NewStyle().Foreground(qmRed)
)

// ── model ─────────────────────────────────────────────────────────────────────

type queueMonitorModel struct {
	client          *downlinkclient.DownlinkClient
	status          downlinkclient.QueueStatus
	spin            spinner.Model
	prog            progress.Model
	vp              viewport.Model
	width           int
	height          int
	maxSeen         int
	actionMsg       string
	actionErr       bool
	taskSteps       []taskStep
	taskMap         map[string]int // taskName → index in taskSteps
	activeArticleId string
	cursor          int
	streamErr       string
}

func newQueueMonitorModel(client *downlinkclient.DownlinkClient) queueMonitorModel {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	pr := progress.New(
		progress.WithDefaultBlend(),
		progress.WithoutPercentage(),
		progress.WithWidth(40),
	)
	return queueMonitorModel{
		client:  client,
		spin:    sp,
		prog:    pr,
		vp:      viewport.New(),
		taskMap: make(map[string]int),
	}
}

func (m queueMonitorModel) Init() tea.Cmd {
	return tea.Batch(
		tea.Cmd(m.spin.Tick),
		cmdFetchStatus(m.client),
		cmdPollTick(),
	)
}

// ── commands ──────────────────────────────────────────────────────────────────

func cmdFetchStatus(c *downlinkclient.DownlinkClient) tea.Cmd {
	return func() tea.Msg {
		return queueDataMsg(c.GetQueueStatus())
	}
}

func cmdPollTick() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg {
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

func cmdDequeueArticle(c *downlinkclient.DownlinkClient, articleId string) tea.Cmd {
	return func() tea.Msg {
		if err := c.DequeueArticle(articleId); err != nil {
			return queueActionMsg("error: " + err.Error())
		}
		return queueActionMsg("✓ removed from queue")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// overheadLines returns the number of terminal lines used outside the viewport.
func overheadLines(hasTaskStrip, hasActionMsg bool) int {
	// \n + title + sep + \n + now + \n + progress + \n + hdr + hdr-sep + \n + help
	n := 12
	if hasTaskStrip {
		n += 2 // strip line + blank
	}
	if hasActionMsg {
		n += 2 // msg line + blank
	}
	return n
}

// syncViewport rebuilds the viewport content and adjusts scroll to keep cursor visible.
func (m *queueMonitorModel) syncViewport() {
	if m.width == 0 {
		return
	}
	titleColW := max(m.width-54, 20)

	// rebuild rows
	content := buildQueueRows(m.status.Queue, m.cursor, titleColW)
	m.vp.SetContent(content)

	// resize viewport
	hasStrip := len(m.taskSteps) > 0 && m.status.IsProcessing
	vpH := max(m.height-overheadLines(hasStrip, m.actionMsg != ""), 3)
	m.vp.SetHeight(vpH)
	m.vp.SetWidth(m.width)

	// scroll to keep cursor visible
	if len(m.status.Queue) > 0 {
		if m.cursor < m.vp.YOffset() {
			m.vp.SetYOffset(m.cursor)
		} else if m.cursor >= m.vp.YOffset()+m.vp.Height() {
			m.vp.SetYOffset(m.cursor - m.vp.Height() + 1)
		}
	}
}

// buildQueueRows renders the full queue as a string for viewport content.
func buildQueueRows(queue []downlinkclient.QueueJobWithTitle, cursor, titleColW int) string {
	if len(queue) == 0 {
		return ""
	}
	var b strings.Builder
	contentFmt := fmt.Sprintf("%%-%ds  %%-14s  %%-20s", titleColW)
	for i, j := range queue {
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
		numStr := fmt.Sprintf("%3d", i+1)
		content := fmt.Sprintf(contentFmt,
			truncate(j.ArticleTitle, titleColW),
			truncate(profile, 14),
			truncate(model, 20),
		)
		if i == cursor {
			b.WriteString(qmSelected.Render("▶ "+numStr+"  "+content) + "\n")
		} else {
			b.WriteString("  " + numStr + "  " + content + "\n")
		}
	}
	return b.String()
}

// applyQueueStatus updates the model for a fresh queue snapshot.
func (m queueMonitorModel) applyQueueStatus(s downlinkclient.QueueStatus) queueMonitorModel {
	if !s.IsProcessing || s.CurrentId != m.activeArticleId {
		m.activeArticleId = s.CurrentId
		if !s.IsProcessing {
			m.activeArticleId = ""
		}
		m.taskSteps = nil
		m.taskMap = make(map[string]int)
	}
	m.status = s
	total := len(s.Queue)
	if s.IsProcessing {
		total++
	}
	if total > m.maxSeen {
		m.maxSeen = total
	}
	if len(s.Queue) > 0 {
		m.cursor = min(m.cursor, len(s.Queue)-1)
	} else {
		m.cursor = 0
	}
	m.syncViewport()
	return m
}

// ── update ────────────────────────────────────────────────────────────────────

func (m queueMonitorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.prog.SetWidth(max(msg.Width-30, 10))
		m.syncViewport()
		return m, nil

	case queueDataMsg:
		m = m.applyQueueStatus(downlinkclient.QueueStatus(msg))
		return m, cmdPollTick()

	case queueStreamUpdateMsg:
		m.streamErr = "" // clear transient error on successful stream event
		m = m.applyQueueStatus(downlinkclient.QueueStatus(msg))
		return m, nil

	case queuePollMsg:
		return m, cmdFetchStatus(m.client)

	case queueProgressMsg:
		ap := downlinkclient.AnalysisProgress(msg)
		if m.taskSteps == nil && ap.TotalTasks > 0 {
			m.taskSteps = make([]taskStep, ap.TotalTasks)
			m.taskMap = make(map[string]int)
		}
		if ap.TaskIndex > 0 && ap.TaskIndex <= len(m.taskSteps) {
			idx := ap.TaskIndex - 1
			if _, known := m.taskMap[ap.TaskName]; !known && ap.TaskName != "" {
				m.taskMap[ap.TaskName] = idx
				m.taskSteps[idx].name = ap.TaskName
			}
			switch ap.Status {
			case "started":
				m.taskSteps[idx].status = taskActive
			case "completed":
				m.taskSteps[idx].status = taskCompleted
			case "error":
				m.taskSteps[idx].status = taskError
			}
		}
		m.syncViewport() // may need to resize if task strip just appeared
		return m, nil

	case queueStreamErrMsg:
		m.streamErr = msg.err.Error()
		return m, nil

	case queueActionMsg:
		s := string(msg)
		m.actionErr = strings.HasPrefix(s, "error:")
		m.actionMsg = s
		m.syncViewport()
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
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.syncViewport()
			}
			return m, nil
		case "down", "j":
			if m.cursor < len(m.status.Queue)-1 {
				m.cursor++
				m.syncViewport()
			}
			return m, nil
		case "d":
			if m.cursor < len(m.status.Queue) {
				id := m.status.Queue[m.cursor].ArticleId
				m.actionMsg = ""
				return m, cmdDequeueArticle(m.client, id)
			}
		}
	}
	return m, nil
}

// ── view ──────────────────────────────────────────────────────────────────────

func (m queueMonitorModel) View() tea.View {
	if m.width == 0 {
		return tea.NewView("\n  Loading…\n")
	}

	var b strings.Builder
	w := m.width

	// ── header ──
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

	// ── current article ──
	if m.status.IsProcessing {
		cur := m.status.CurrentTitle
		if cur == "" {
			cur = "processing…"
		}
		b.WriteString("  " + qmDim.Render("now") + "   " + m.spin.View() + " " + qmCurrent.Render(truncate(cur, max(w-14, 10))) + "\n\n")
	} else {
		b.WriteString("  " + qmDim.Render("now") + "   " + qmDim.Render("—") + "\n\n")
	}

	// ── progress bar ──
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

	// ── task strip ──
	if len(m.taskSteps) > 0 && m.status.IsProcessing {
		var parts []string
		for _, step := range m.taskSteps {
			if step.name == "" {
				continue
			}
			var label string
			switch step.status {
			case taskActive:
				label = qmCurrent.Render(step.name) + " " + m.spin.View()
			case taskCompleted:
				label = qmStepOk.Render(step.name + " ✓")
			case taskError:
				label = qmStepErr.Render(step.name + " ✗")
			default:
				label = qmDim.Render(step.name + " ○")
			}
			parts = append(parts, label)
		}
		if len(parts) > 0 {
			b.WriteString("  " + strings.Join(parts, qmDim.Render("  ·  ")) + "\n\n")
		}
	}

	// ── queue table ──
	titleColW := max(w-54, 20)
	colFmt := fmt.Sprintf("  %%3s  %%-%ds  %%-14s  %%-20s", titleColW)

	if len(m.status.Queue) > 0 {
		b.WriteString(qmColHeader.Render(fmt.Sprintf(colFmt, "#", "TITLE", "PROFILE", "MODEL")) + "\n")
		b.WriteString(qmDim.Render(fmt.Sprintf(colFmt,
			"───",
			strings.Repeat("─", titleColW),
			strings.Repeat("─", 14),
			strings.Repeat("─", 20),
		)) + "\n")
		b.WriteString(m.vp.View())
	} else if !m.status.IsProcessing {
		b.WriteString("  " + qmDim.Render("Queue is empty") + "\n")
	}
	b.WriteString("\n")

	// ── action feedback ──
	if m.actionMsg != "" {
		if m.actionErr {
			b.WriteString("  " + qmErrStyle.Render(m.actionMsg) + "\n\n")
		} else {
			b.WriteString("  " + qmOk.Render(m.actionMsg) + "\n\n")
		}
	}

	// ── stream error ──
	if m.streamErr != "" {
		b.WriteString("  " + qmWarn.Render("⚠  "+m.streamErr) + "\n\n")
	}

	// ── help ──
	help := qmKey.Render("s") + qmDim.Render(" start") + "   " +
		qmKey.Render("x") + qmDim.Render(" stop") + "   " +
		qmKey.Render("c") + qmDim.Render(" clear") + "   " +
		qmKey.Render("↑↓") + "/" + qmKey.Render("jk") + qmDim.Render(" select") + "   " +
		qmKey.Render("d") + qmDim.Render(" dequeue") + "   " +
		qmKey.Render("q") + qmDim.Render(" quit")
	b.WriteString("  " + help + "\n")

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// ── entry point ───────────────────────────────────────────────────────────────

func runQueueMonitor(client *downlinkclient.DownlinkClient) error {
	m := newQueueMonitorModel(client)
	p := tea.NewProgram(m)

	go client.StreamQueueEvents(downlinkclient.QueueStreamCallbacks{
		OnQueueUpdate: func(s downlinkclient.QueueStatus) {
			p.Send(queueStreamUpdateMsg(s))
		},
		OnAnalysisProgress: func(a downlinkclient.AnalysisProgress) {
			p.Send(queueProgressMsg(a))
		},
		OnDisconnect: func(e error) {
			p.Send(queueStreamErrMsg{err: e})
		},
	})

	_, err := p.Run()
	return err
}
