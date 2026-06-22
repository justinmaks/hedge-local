package views

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/justinmaks/hedge-local/internal/tui"
	"github.com/justinmaks/hedge-local/internal/tui/queries"
)

type ProjectsView struct {
	service     *queries.Service
	stats       []queries.ProjectStats
	table       *tui.Table
	detailTable *tui.Table
	focused     bool
	selected    string
	from        time.Time
	to          time.Time
	err         error
}

func NewProjectsView(service *queries.Service) *ProjectsView {
	tbl := tui.NewTable([]tui.Column{
		{Title: "Project", Width: 20},
		{Title: "Sessions", Width: 9},
		{Title: "Cost", Width: 10},
		{Title: "Tokens", Width: 10},
		{Title: "Last Active", Width: 16},
	})
	detailTbl := tui.NewTable([]tui.Column{
		{Title: "Started", Width: 14},
		{Title: "Agent", Width: 12},
		{Title: "Cost", Width: 10},
		{Title: "Tokens", Width: 10},
		{Title: "Tools", Width: 6},
	})
	return &ProjectsView{service: service, table: tbl, detailTable: detailTbl}
}

func (v *ProjectsView) Title() string { return "Projects" }

func (v *ProjectsView) Init() tea.Cmd { return nil }

func (v *ProjectsView) Reload(ctx tui.ViewContext) tea.Cmd {
	v.from = ctx.From
	v.to = ctx.To
	return func() tea.Msg {
		stats, err := v.service.ProjectSummary(ctx.From, ctx.To)
		return projectsLoadedMsg{stats: stats, err: err}
	}
}

func (v *ProjectsView) Hints() string {
	if v.focused {
		return "esc back  ↑↓ scroll"
	}
	return "↑↓ scroll  enter sessions"
}

type projectsLoadedMsg struct {
	stats []queries.ProjectStats
	err   error
}

func (v *ProjectsView) Update(msg tea.Msg, ctx tui.ViewContext) (tui.View, tea.Cmd) {
	switch m := msg.(type) {
	case projectsLoadedMsg:
		v.stats = m.stats
		v.err = m.err
		v.refreshTable()
		if v.focused && v.selected != "" {
			v.loadDetail(v.selected)
		}
		return v, nil
	case tea.KeyMsg:
		if v.focused {
			switch m.String() {
			case "esc":
				v.focused = false
			case "up":
				v.detailTable.ScrollUp()
			case "down":
				v.detailTable.ScrollDown()
			}
			return v, nil
		}

		switch m.String() {
		case "up":
			v.table.ScrollUp()
		case "down":
			v.table.ScrollDown()
		case "enter":
			if v.table.Cursor() < len(v.stats) {
				v.focused = true
				v.selected = v.stats[v.table.Cursor()].Path
				v.loadDetail(v.selected)
			}
		}
	}

	return v, nil
}

func (v *ProjectsView) refreshTable() {
	rows := make([][]string, 0, len(v.stats))
	for _, ps := range v.stats {
		name := ps.Name
		if name == "" {
			name = truncate(ps.Path, 20)
		}

		lastActive := "-"
		if !ps.LastActive.IsZero() {
			lastActive = ps.LastActive.Format("01-02 15:04")
		}

		rows = append(rows, []string{
			name,
			fmt.Sprintf("%d", ps.Sessions),
			fmt.Sprintf("$%.2f", ps.Cost),
			fmt.Sprintf("%d", ps.Tokens),
			lastActive,
		})
	}
	v.table.SetRows(rows)
}

func (v *ProjectsView) loadDetail(projectPath string) {
	sessions, err := v.service.RecentSessionsInRange(projectPath, 20, v.from, v.to)
	if err != nil {
		v.detailTable.SetRows([][]string{{"Error: " + err.Error()}})
		return
	}

	rows := make([][]string, 0, len(sessions))
	for _, s := range sessions {
		rows = append(rows, []string{
			s.StartedAt.Format("01-02 15:04"),
			s.Agent,
			fmt.Sprintf("$%.2f", s.Cost),
			fmt.Sprintf("%d", s.Tokens),
			fmt.Sprintf("%d", s.ToolCount),
		})
	}
	v.detailTable.SetRows(rows)
}

func (v *ProjectsView) Render(width, height int, theme *tui.Theme) string {
	if v.err != nil {
		return theme.ErrorMsg.Render("Error: " + v.err.Error())
	}
	if v.focused {
		return theme.Header.Render("Sessions (esc to return)") + "\n" + v.detailTable.Render(width, height-2, theme)
	}
	return v.table.Render(width, height, theme)
}
