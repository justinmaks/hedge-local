package views

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/justinmaks/hedge-local/internal/tui"
	"github.com/justinmaks/hedge-local/internal/tui/queries"
)

var dataTypes = []string{"Sessions", "LLM Calls", "Tool Calls", "Events", "Weekly Report"}
var formats = []string{"CSV", "JSON", "Markdown"}
var destinations = []string{"File", "Clipboard", "Stdout"}

type ExportView struct {
	service        *queries.Service
	activeSelector int
	dataType       int
	format         int
	destination    int
	preview        string
	result         string
	err            error
	now            func() time.Time
	writeFile      func(string, []byte) error
	copyText       func(string) error
}

type exportPreviewMsg struct {
	preview string
	err     error
}

func NewExportView(service *queries.Service) *ExportView {
	view := &ExportView{
		service:   service,
		now:       time.Now,
		writeFile: func(name string, data []byte) error { return os.WriteFile(name, data, 0o644) },
	}
	view.copyText = view.copyToClipboard
	return view
}

func (v *ExportView) Title() string { return "Export" }

func (v *ExportView) Init() tea.Cmd { return nil }

func (v *ExportView) Reload(ctx tui.ViewContext) tea.Cmd {
	return v.previewCmd(ctx.From, ctx.To)
}

func (v *ExportView) Hints() string {
	return "←/→ field  ↑/↓ value  p preview  x export"
}

func (v *ExportView) Update(msg tea.Msg, ctx tui.ViewContext) (tui.View, tea.Cmd) {
	switch m := msg.(type) {
	case exportPreviewMsg:
		v.preview = m.preview
		v.err = m.err
		return v, nil
	case tea.KeyMsg:
		switch m.String() {
		case "left":
			v.moveSelector(-1)
		case "right":
			v.moveSelector(1)
		case "up":
			if v.cycleValue(-1) {
				return v, v.previewCmd(ctx.From, ctx.To)
			}
		case "down":
			if v.cycleValue(1) {
				return v, v.previewCmd(ctx.From, ctx.To)
			}
		case "p":
			v.doPreview(ctx.From, ctx.To)
		case "x":
			v.doExport(ctx.From, ctx.To)
		}
	}
	return v, nil
}

func (v *ExportView) moveSelector(dir int) {
	v.activeSelector = cycleIndex(v.activeSelector, dir, 3)
}

func (v *ExportView) cycleValue(dir int) bool {
	switch v.activeSelector {
	case 0:
		v.dataType = cycleIndex(v.dataType, dir, len(dataTypes))
		return true
	case 1:
		v.format = cycleIndex(v.format, dir, len(formats))
		return true
	case 2:
		v.destination = cycleIndex(v.destination, dir, len(destinations))
	}
	return false
}

func cycleIndex(current, dir, total int) int {
	if total <= 0 {
		return 0
	}
	next := (current + dir) % total
	if next < 0 {
		next += total
	}
	return next
}

func (v *ExportView) getHeaders() []string {
	switch v.dataType {
	case 0:
		return []string{"session_id", "agent", "project", "started_at", "ended_at", "cost_usd", "input_tokens", "output_tokens", "tool_calls", "message_count"}
	case 1:
		return []string{"started_at", "agent", "model", "provider", "input_tokens", "output_tokens", "cache_read", "cache_write", "cost_usd", "duration_ms", "stop_reason"}
	case 2:
		return []string{"started_at", "agent", "tool_name", "success", "duration_ms", "error_message"}
	case 3:
		return []string{"timestamp", "agent", "event_name", "payload"}
	default:
		return nil
	}
}

func (v *ExportView) getRows(from, to time.Time) ([][]string, error) {
	if v.service == nil {
		return nil, fmt.Errorf("export service is not configured")
	}

	switch v.dataType {
	case 0:
		rows, err := v.service.ExportSessions(from, to)
		if err != nil {
			return nil, err
		}
		result := make([][]string, 0, len(rows))
		for _, r := range rows {
			endedAt := ""
			if r.EndedAt != nil {
				endedAt = r.EndedAt.Format(time.RFC3339)
			}
			result = append(result, []string{
				r.ExternalID,
				r.Agent,
				r.ProjectPath,
				r.StartedAt.Format(time.RFC3339),
				endedAt,
				fmt.Sprintf("%.4f", r.TotalCostUSD),
				fmt.Sprintf("%d", r.InputTokens),
				fmt.Sprintf("%d", r.OutputTokens),
				fmt.Sprintf("%d", r.ToolCallCount),
				fmt.Sprintf("%d", r.MessageCount),
			})
		}
		return result, nil
	case 1:
		rows, err := v.service.ExportLLMCalls(from, to)
		if err != nil {
			return nil, err
		}
		result := make([][]string, 0, len(rows))
		for _, r := range rows {
			result = append(result, []string{
				r.StartedAt.Format(time.RFC3339),
				r.Agent,
				r.Model,
				r.Provider,
				fmt.Sprintf("%d", r.InputTokens),
				fmt.Sprintf("%d", r.OutputTokens),
				fmt.Sprintf("%d", r.CacheReadTokens),
				fmt.Sprintf("%d", r.CacheWriteTokens),
				fmt.Sprintf("%.4f", r.CostUSD),
				fmt.Sprintf("%d", r.DurationMs),
				r.StopReason,
			})
		}
		return result, nil
	case 2:
		rows, err := v.service.ExportToolCalls(from, to)
		if err != nil {
			return nil, err
		}
		result := make([][]string, 0, len(rows))
		for _, r := range rows {
			result = append(result, []string{
				r.StartedAt.Format(time.RFC3339),
				r.Agent,
				r.ToolName,
				fmt.Sprintf("%t", r.Success),
				fmt.Sprintf("%d", r.DurationMs),
				r.ErrorMessage,
			})
		}
		return result, nil
	case 3:
		rows, err := v.service.ExportEvents(from, to)
		if err != nil {
			return nil, err
		}
		result := make([][]string, 0, len(rows))
		for _, r := range rows {
			result = append(result, []string{
				r.Timestamp.Format(time.RFC3339),
				r.Agent,
				r.EventName,
				r.Payload,
			})
		}
		return result, nil
	default:
		return nil, nil
	}
}

func (v *ExportView) doPreview(from, to time.Time) {
	v.result = ""
	preview, err := v.buildPreview(from, to)
	if err != nil {
		v.err = err
		return
	}
	v.preview = preview
	v.err = nil
}

func (v *ExportView) previewCmd(from, to time.Time) tea.Cmd {
	return func() tea.Msg {
		preview, err := v.buildPreview(from, to)
		return exportPreviewMsg{preview: preview, err: err}
	}
}

func (v *ExportView) buildPreview(from, to time.Time) (string, error) {
	if v.dataType == 4 {
		return "Weekly report preview not available; export to see the full markdown output.", nil
	}

	headers := v.getHeaders()
	rows, err := v.getRows(from, to)
	if err != nil {
		return "", err
	}
	if len(rows) > 10 {
		rows = rows[:10]
	}

	content, err := v.renderExport(headers, rows)
	if err != nil {
		return "", err
	}
	return content, nil
}

func (v *ExportView) doExport(from, to time.Time) {
	v.result = ""
	var (
		content string
		err     error
		count   int
	)

	if v.dataType == 4 {
		content, err = v.renderWeeklyReport(from, to)
		count = 1
	} else {
		var rows [][]string
		rows, err = v.getRows(from, to)
		if err == nil {
			count = len(rows)
			content, err = v.renderExport(v.getHeaders(), rows)
		}
	}
	if err != nil {
		v.err = err
		return
	}

	switch v.destination {
	case 0:
		name := v.defaultFilename()
		if err := v.writeFile(name, []byte(content)); err != nil {
			v.err = fmt.Errorf("write export file: %w", err)
			return
		}
		v.result = "Wrote " + name
	case 1:
		if err := v.copyText(content); err != nil {
			v.err = err
			return
		}
		v.result = "Copied to clipboard"
	default:
		v.result = content
	}

	v.err = nil
	if v.destination != 2 && v.dataType != 4 {
		v.result = fmt.Sprintf("%s (%d rows)", v.result, count)
	}
	if v.destination != 2 && v.dataType == 4 {
		v.result = v.result + " (weekly report)"
	}
	if v.destination == 2 && v.dataType != 4 && count == 0 && v.result == "" {
		v.result = fmt.Sprintf("Exported %d rows", count)
	}
	if v.destination == 2 && v.dataType == 4 && v.result == "" {
		v.result = "Weekly report exported"
	}
}

func (v *ExportView) renderExport(headers []string, rows [][]string) (string, error) {
	var buf bytes.Buffer
	switch v.format {
	case 0:
		if err := tui.WriteCSV(&buf, headers, rows); err != nil {
			return "", err
		}
	case 1:
		if err := tui.WriteJSON(&buf, rowsAsObjects(headers, rows)); err != nil {
			return "", err
		}
	default:
		if err := tui.WriteMarkdown(&buf, headers, rows); err != nil {
			return "", err
		}
	}
	return buf.String(), nil
}

func rowsAsObjects(headers []string, rows [][]string) []map[string]string {
	result := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		item := make(map[string]string, len(headers))
		for i, header := range headers {
			if i < len(row) {
				item[header] = row[i]
			}
		}
		result = append(result, item)
	}
	return result
}

func (v *ExportView) renderWeeklyReport(from, to time.Time) (string, error) {
	if v.service == nil {
		return "", fmt.Errorf("export service is not configured")
	}

	overview, err := v.service.OverviewSummary(from, to)
	if err != nil {
		return "", err
	}
	projects, err := v.service.ProjectSummary(from, to)
	if err != nil {
		return "", err
	}
	tools, err := v.service.ToolSummary(from, to)
	if err != nil {
		return "", err
	}
	agents, err := v.service.CostByDimension(from, to, "agent")
	if err != nil {
		return "", err
	}

	stats := tui.WeeklyReportStats{
		FromDate:    from.Format("2006-01-02"),
		ToDate:      to.Format("2006-01-02"),
		TotalCost:   overview.TodayCost,
		TotalTokens: overview.TodayTokens,
		Sessions:    overview.TodaySessions,
		TopProjects: make([][]string, 0, minInt(len(projects), 5)),
		TopTools:    make([][]string, 0, minInt(len(tools), 5)),
		ByAgent:     make([][]string, 0, minInt(len(agents), 5)),
	}

	for _, project := range projects[:minInt(len(projects), 5)] {
		name := project.Name
		if name == "" {
			name = project.Path
		}
		if name == "" {
			name = "(unknown)"
		}
		stats.TopProjects = append(stats.TopProjects, []string{
			name,
			fmt.Sprintf("$%.2f", project.Cost),
			fmt.Sprintf("%d", project.Sessions),
		})
	}

	for _, tool := range tools[:minInt(len(tools), 5)] {
		stats.TopTools = append(stats.TopTools, []string{
			tool.ToolName,
			fmt.Sprintf("%d", tool.Calls),
			fmt.Sprintf("%.1f", tool.SuccessRate),
			fmt.Sprintf("$%.2f", tool.TotalCost),
		})
	}

	for _, agent := range agents[:minInt(len(agents), 5)] {
		stats.ByAgent = append(stats.ByAgent, []string{
			agent.Name,
			fmt.Sprintf("$%.2f", agent.Cost),
			fmt.Sprintf("%d", agent.Tokens),
			fmt.Sprintf("%d", agent.Sessions),
		})
	}

	var buf bytes.Buffer
	if err := tui.WriteWeeklyReport(&buf, stats); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (v *ExportView) defaultFilename() string {
	return fmt.Sprintf("hcli-%s-%s.%s", v.dataSlug(), v.now().Format("20060102-150405"), v.fileExtension())
}

func (v *ExportView) dataSlug() string {
	switch v.dataType {
	case 0:
		return "sessions"
	case 1:
		return "llm-calls"
	case 2:
		return "tool-calls"
	case 3:
		return "events"
	default:
		return "weekly-report"
	}
}

func (v *ExportView) fileExtension() string {
	if v.dataType == 4 {
		return "md"
	}
	switch v.format {
	case 0:
		return "csv"
	case 1:
		return "json"
	default:
		return "md"
	}
}

func (v *ExportView) copyToClipboard(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else {
			return fmt.Errorf("clipboard support requires wl-copy or xclip")
		}
	case "windows":
		cmd = exec.Command("clip")
	default:
		return fmt.Errorf("clipboard not supported on %s", runtime.GOOS)
	}

	cmd.Stdin = bytes.NewBufferString(text)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("copy to clipboard: %w", err)
	}
	return nil
}

func (v *ExportView) Render(width, height int, theme *tui.Theme) string {
	if v.err != nil {
		return theme.ErrorMsg.Render("Error: " + v.err.Error())
	}

	selector := strings.Join([]string{
		renderSelector("Data", dataTypes[v.dataType], v.activeSelector == 0),
		renderSelector("Format", formats[v.format], v.activeSelector == 1),
		renderSelector("To", destinations[v.destination], v.activeSelector == 2),
	}, "  |  ")

	content := theme.Header.Render(selector) + "\n\n"
	if v.dataType == 4 {
		content += theme.HelpText.Render("Weekly report exports as markdown regardless of the selected format.") + "\n\n"
	}
	if v.preview != "" {
		content += theme.CardTitle.Render("Preview") + "\n"
		content += v.preview + "\n\n"
	}
	if v.result != "" {
		content += theme.Focused.Render(v.result) + "\n\n"
	}
	content += theme.HelpText.Render(v.Hints())
	return content
}

func renderSelector(label, value string, active bool) string {
	if active {
		return fmt.Sprintf("%s: [%s]", label, value)
	}
	return fmt.Sprintf("%s: %s", label, value)
}
