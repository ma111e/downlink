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


// ── messages ──────────────────────────────────────────────────────────────────

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

type activeJob struct {
	title     string
	taskSteps []taskStep
	taskMap   map[string]int
	done      bool
}

// allTasksDone returns true when every named slot has completed or errored.
func (j *activeJob) allTasksDone() bool {
	if len(j.taskSteps) == 0 {
		return false
	}
	for _, s := range j.taskSteps {
		if s.name != "" && s.status != taskCompleted && s.status != taskError {
			return false
		}
	}
	return true
}


// ── model ─────────────────────────────────────────────────────────────────────

type queueMonitorModel struct {
	client         *downlinkclient.DownlinkClient
	status         downlinkclient.QueueStatus
	spin           spinner.Model
	prog           progress.Model
	width          int
	height         int
	maxSeen        int
	actionMsg      string
	actionErr      bool
	activeJobs     map[string]*activeJob // articleId → in-flight job
	activeJobOrder []string              // insertion-order for stable display
	titleCache     map[string]string     // articleId → title
	cursor         int
	streamErr      string
}

func newQueueMonitorModel(client *downlinkclient.DownlinkClient) queueMonitorModel {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	pr := progress.New(
		progress.WithDefaultBlend(),
		progress.WithoutPercentage(),
		progress.WithWidth(40),
	)
	return queueMonitorModel{
		client:     client,
		spin:       sp,
		prog:       pr,
		activeJobs: make(map[string]*activeJob),
		titleCache: make(map[string]string),
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

// applyQueueStatus applies a fresh queue snapshot to the model.
func (m queueMonitorModel) applyQueueStatus(s downlinkclient.QueueStatus) queueMonitorModel {
	// update title cache
	for _, j := range s.Queue {
		if j.ArticleTitle != "" {
			m.titleCache[j.ArticleId] = j.ArticleTitle
		}
	}
	if s.CurrentId != "" && s.CurrentTitle != "" {
		m.titleCache[s.CurrentId] = s.CurrentTitle
	}

	// backfill titles on jobs that were created before a title was known
	for id, job := range m.activeJobs {
		if job.title == "" {
			if t, ok := m.titleCache[id]; ok {
				job.title = t
			}
		}
	}

	// prune done jobs; next queue_update is the signal they've cleared
	newOrder := m.activeJobOrder[:0]
	for _, id := range m.activeJobOrder {
		if job, ok := m.activeJobs[id]; ok && job.done {
			delete(m.activeJobs, id)
		} else {
			newOrder = append(newOrder, id)
		}
	}
	m.activeJobOrder = newOrder

	m.status = s

	total := len(s.Queue) + len(m.activeJobs)
	if total > m.maxSeen {
		m.maxSeen = total
	}

	if len(s.Queue) > 0 {
		m.cursor = min(m.cursor, len(s.Queue)-1)
	} else {
		m.cursor = 0
	}
	return m
}

// ── update ────────────────────────────────────────────────────────────────────

func (m queueMonitorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.prog.SetWidth(max(msg.Width-30, 10))
		return m, nil

	case queueDataMsg:
		m = m.applyQueueStatus(downlinkclient.QueueStatus(msg))
		return m, cmdPollTick()

	case queueStreamUpdateMsg:
		m.streamErr = ""
		m = m.applyQueueStatus(downlinkclient.QueueStatus(msg))
		return m, nil

	case queuePollMsg:
		return m, cmdFetchStatus(m.client)

	case queueProgressMsg:
		ap := downlinkclient.AnalysisProgress(msg)
		if ap.Status == "token" || ap.ArticleId == "" {
			return m, nil
		}

		job, exists := m.activeJobs[ap.ArticleId]
		if !exists {
			job = &activeJob{taskMap: make(map[string]int)}
			if t, ok := m.titleCache[ap.ArticleId]; ok {
				job.title = t
			}
			m.activeJobs[ap.ArticleId] = job
			m.activeJobOrder = append(m.activeJobOrder, ap.ArticleId)
		}

		if ap.TotalTasks > 0 && len(job.taskSteps) < ap.TotalTasks {
			job.taskSteps = append(job.taskSteps, make([]taskStep, ap.TotalTasks-len(job.taskSteps))...)
		}

		if ap.TaskIndex > 0 && ap.TaskIndex <= len(job.taskSteps) {
			idx := ap.TaskIndex - 1
			if ap.TaskName != "" {
				job.taskSteps[idx].name = ap.TaskName
				job.taskMap[ap.TaskName] = idx
			}
			switch ap.Status {
			case "started":
				job.taskSteps[idx].status = taskActive
			case "completed":
				job.taskSteps[idx].status = taskCompleted
			case "error":
				job.taskSteps[idx].status = taskError
			}
		}

		if job.allTasksDone() {
			job.done = true
		}
		return m, nil

	case queueStreamErrMsg:
		m.streamErr = msg.err.Error()
		return m, nil

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
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down", "j":
			if m.cursor < len(m.status.Queue)-1 {
				m.cursor++
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
	titleStr := styleBold.Render("Analysis Queue")
	var stateStr string
	if m.status.IsProcessing {
		stateStr = styleActive.Render("● PROCESSING")
	} else {
		stateStr = styleDim.Render("○  IDLE")
	}
	gap := max(w-lipgloss.Width(titleStr)-lipgloss.Width(stateStr)-4, 1)
	b.WriteString("  " + titleStr + strings.Repeat(" ", gap) + stateStr + "\n")
	b.WriteString("  " + styleDim.Render(strings.Repeat("─", w-4)) + "\n\n")

	// ── progress bar ──
	activeCount := len(m.activeJobs)
	pending := len(m.status.Queue)
	total := activeCount + pending
	if total > m.maxSeen {
		m.maxSeen = total
	}
	var pct float64
	var countStr string
	if m.maxSeen > 0 {
		pct = 1.0 - float64(total)/float64(m.maxSeen)
		countStr = fmt.Sprintf("%d / %d", m.maxSeen-total, m.maxSeen)
	} else if !m.status.IsProcessing {
		countStr = "idle"
	} else {
		countStr = "starting…"
	}
	b.WriteString("  " + m.prog.ViewAs(pct) + "  " + styleDim.Render(countStr) + "\n\n")

	// ── active section ──
	if len(m.activeJobOrder) > 0 {
		sectionSep := strings.Repeat("─", max(w-14, 4))
		b.WriteString("  " + styleSection.Render("Active") + "  " + styleDim.Render(sectionSep) + "\n")

		for _, id := range m.activeJobOrder {
			job, ok := m.activeJobs[id]
			if !ok || job.done {
				continue
			}
			title := job.title
			if title == "" {
				title = id
			}
			b.WriteString("  " + styleCurrent.Render(truncate(title, max(w-4, 20))) + "\n")

			// task strip
			var parts []string
			hasAny := false
			for _, step := range job.taskSteps {
				if step.name == "" {
					continue
				}
				hasAny = true
				var label string
				switch step.status {
				case taskActive:
					label = styleCurrent.Render(step.name) + " " + m.spin.View()
				case taskCompleted:
					label = styleOK.Render(step.name + " ✓")
				case taskError:
					label = styleErr.Render(step.name + " ✗")
				default:
					label = styleDim.Render(step.name + " ○")
				}
				parts = append(parts, label)
			}
			if hasAny {
				b.WriteString("    " + strings.Join(parts, styleDim.Render("  ·  ")) + "\n")
			} else {
				b.WriteString("    " + m.spin.View() + " " + styleDim.Render("waiting for tasks…") + "\n")
			}
			b.WriteString("\n")
		}
	}

	// ── queued section ──
	if pending > 0 || (len(m.activeJobOrder) > 0) {
		sectionSep := strings.Repeat("─", max(w-13, 4))
		b.WriteString("  " + styleSection.Render("Queued") + "  " + styleDim.Render(sectionSep) + "\n")
	}

	if pending > 0 {
		titleColW := max(w-54, 20)
		colFmt := fmt.Sprintf("  %%3s  %%-%ds  %%-14s  %%-20s", titleColW)
		b.WriteString(styleColHdr.Render(fmt.Sprintf(colFmt, "#", "TITLE", "PROFILE", "MODEL")) + "\n")
		b.WriteString(styleDim.Render(fmt.Sprintf(colFmt,
			"───",
			strings.Repeat("─", titleColW),
			strings.Repeat("─", 14),
			strings.Repeat("─", 20),
		)) + "\n")
		contentFmt := fmt.Sprintf("%%-%ds  %%-14s  %%-20s", titleColW)
		for i, j := range m.status.Queue {
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
			if i == m.cursor {
				b.WriteString(styleSelected.Render("▶ "+numStr+"  "+content) + "\n")
			} else {
				b.WriteString("  " + numStr + "  " + content + "\n")
			}
		}
	} else if !m.status.IsProcessing && len(m.activeJobOrder) == 0 {
		b.WriteString("  " + styleDim.Render("Queue is empty") + "\n")
	} else if pending == 0 && m.status.IsProcessing {
		b.WriteString("  " + styleDim.Render("(all jobs in flight)") + "\n")
	}
	b.WriteString("\n")

	// ── action feedback ──
	if m.actionMsg != "" {
		if m.actionErr {
			b.WriteString("  " + styleErr.Render(m.actionMsg) + "\n\n")
		} else {
			b.WriteString("  " + styleOK.Render(m.actionMsg) + "\n\n")
		}
	}

	// ── stream error ──
	if m.streamErr != "" {
		b.WriteString("  " + styleWarn.Render("⚠  "+m.streamErr) + "\n\n")
	}

	// ── help ──
	help := styleKey.Render("s") + styleDim.Render(" start") + "   " +
		styleKey.Render("x") + styleDim.Render(" stop") + "   " +
		styleKey.Render("c") + styleDim.Render(" clear") + "   " +
		styleKey.Render("↑↓") + "/" + styleKey.Render("jk") + styleDim.Render(" select") + "   " +
		styleKey.Render("d") + styleDim.Render(" dequeue") + "   " +
		styleKey.Render("q") + styleDim.Render(" quit")
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
