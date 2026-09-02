package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"pc-state-mqtt/internal/config"
	"pc-state-mqtt/internal/systemstate"
	"pc-state-mqtt/pkg/telemetry"
)

var (
	titleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("24")).Bold(true)
	enabledStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	valueStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
)

type collectFunc func(context.Context, config.Config, time.Time) telemetry.Snapshot

type Options struct {
	Config     config.Config
	ConfigPath string
	Input      io.Reader
	Output     io.Writer
	Collect    collectFunc
}

type feature struct {
	name        string
	description string
	implemented bool
}

var features = []feature{
	{"CPU", "usage and per-core clocks", true},
	{"Memory", "memory, cache, and swap", true},
	{"Thermal", "temperature, fan, voltage, and power sensors", false},
	{"Storage", "filesystems, block devices, and I/O", false},
	{"Network", "interfaces, addresses, and traffic", false},
	{"GPU", "identity, utilization, clocks, and VRAM", false},
	{"Display", "DRM connectors, EDID, and modes", false},
	{"Hyprland", "logical monitor and workspace state", false},
	{"Docker", "container state and resource usage", false},
}

type viewMode uint8

const (
	settingsView viewMode = iota
	monitorView
)

type model struct {
	ctx           context.Context
	configuration config.Config
	configPath    string
	collect       collectFunc
	mode          viewMode
	cursor        int
	offset        int
	width         int
	height        int
	collecting    bool
	snapshot      telemetry.Snapshot
	hasSnapshot   bool
	status        string
	statusErr     bool
}

type snapshotMsg struct {
	snapshot telemetry.Snapshot
}

type refreshMsg struct{}

type saveMsg struct {
	err error
}

func Run(ctx context.Context, options Options) error {
	if options.Collect == nil {
		options.Collect = systemstate.Collect
	}
	program := tea.NewProgram(
		newModel(ctx, options),
		tea.WithAltScreen(),
		tea.WithContext(ctx),
		tea.WithInput(options.Input),
		tea.WithOutput(options.Output),
	)
	_, err := program.Run()
	return err
}

func newModel(ctx context.Context, options Options) model {
	collector := options.Collect
	if collector == nil {
		collector = systemstate.Collect
	}
	return model{
		ctx: ctx, configuration: options.Config, configPath: options.ConfigPath,
		collect: collector, mode: settingsView,
	}
}

func (model) Init() tea.Cmd { return nil }

func (current model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		current.width, current.height = message.Width, message.Height
		current.clampOffset()
	case tea.KeyMsg:
		if message.String() == "q" || message.String() == "ctrl+c" {
			return current, tea.Quit
		}
		if current.mode == settingsView {
			return current.updateSettings(message)
		}
		return current.updateMonitor(message)
	case snapshotMsg:
		current.snapshot = message.snapshot
		current.hasSnapshot = true
		current.collecting = false
		current.statusErr = false
		current.status = fmt.Sprintf("Updated %s", message.snapshot.ObservedAt.Local().Format("15:04:05"))
		current.clampOffset()
		return current, refreshAfter(current.configuration.SampleInterval.Duration)
	case refreshMsg:
		if current.mode == monitorView && !current.collecting {
			current.collecting = true
			return current, current.collectCmd()
		}
	case saveMsg:
		current.statusErr = message.err != nil
		if message.err != nil {
			current.status = message.err.Error()
		} else {
			current.status = "Saved " + current.configPath
		}
	}
	return current, nil
}

func (current model) updateSettings(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "up", "k":
		if current.cursor > 0 {
			current.cursor--
		}
	case "down", "j":
		if current.cursor < len(features)-1 {
			current.cursor++
		}
	case " ", "enter":
		current.setFeature(current.cursor, !current.featureEnabled(current.cursor))
		current.status = features[current.cursor].name + " setting changed; press s to save"
		current.statusErr = false
	case "s", "ctrl+s":
		path := current.configPath
		configuration := current.configuration
		current.status = "Saving configuration..."
		return current, func() tea.Msg {
			return saveMsg{err: config.SaveCollectors(path, configuration.Collectors)}
		}
	case "m":
		current.mode = monitorView
		current.offset = 0
		current.collecting = true
		current.status = "Collecting system state..."
		return current, current.collectCmd()
	}
	return current, nil
}

func (current model) updateMonitor(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "m":
		current.mode = settingsView
		current.status = "Feature settings"
	case "up", "k":
		if current.offset > 0 {
			current.offset--
		}
	case "down", "j":
		current.offset++
		current.clampOffset()
	case "pgup":
		current.offset -= current.visibleMetricRows()
		current.clampOffset()
	case "pgdown":
		current.offset += current.visibleMetricRows()
		current.clampOffset()
	case "r":
		if !current.collecting {
			current.collecting = true
			current.status = "Refreshing..."
			return current, current.collectCmd()
		}
	}
	return current, nil
}

func (current model) collectCmd() tea.Cmd {
	ctx := current.ctx
	configuration := current.configuration
	collect := current.collect
	return func() tea.Msg {
		return snapshotMsg{snapshot: collect(ctx, configuration, time.Now())}
	}
}

func refreshAfter(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(time.Time) tea.Msg { return refreshMsg{} })
}

func (current model) View() string {
	if current.mode == monitorView {
		return current.monitorView()
	}
	return current.settingsView()
}

func (current model) settingsView() string {
	var view strings.Builder
	view.WriteString(titleStyle.Render("pc-state-mqtt"))
	view.WriteString(mutedStyle.Render("  Feature settings"))
	view.WriteString("\n\n")
	for index, feature := range features {
		checked := "[ ]"
		if current.featureEnabled(index) {
			checked = "[x]"
		}
		state := "planned"
		if feature.implemented {
			state = "available"
		}
		line := fmt.Sprintf("  %s %-10s %-42s %s", checked, feature.name, feature.description, state)
		if index == current.cursor {
			line = selectedStyle.Render("> " + line[2:])
		} else if current.featureEnabled(index) {
			line = enabledStyle.Render(line)
		}
		view.WriteString(line)
		view.WriteByte('\n')
	}
	view.WriteString("\n")
	view.WriteString(mutedStyle.Render("Up/Down select  Space toggle  s save  m monitor  q quit"))
	current.writeStatus(&view)
	return view.String()
}

func (current model) monitorView() string {
	var view strings.Builder
	view.WriteString(titleStyle.Render("pc-state-mqtt"))
	view.WriteString(mutedStyle.Render("  Live monitor"))
	if current.collecting {
		view.WriteString(mutedStyle.Render("  refreshing"))
	}
	view.WriteString("\n\n")
	if !current.hasSnapshot {
		view.WriteString("Collecting system state...\n")
	} else {
		rows := current.metricRows()
		end := min(len(rows), current.offset+current.visibleMetricRows())
		for _, row := range rows[current.offset:end] {
			view.WriteString(row)
			view.WriteByte('\n')
		}
		if len(current.snapshot.Diagnostics) > 0 {
			view.WriteString("\n")
			for _, diagnostic := range current.snapshot.Diagnostics {
				view.WriteString(errorStyle.Render(diagnostic.Collector + ": " + diagnostic.Error))
				view.WriteByte('\n')
			}
		}
	}
	view.WriteString("\n")
	view.WriteString(mutedStyle.Render("Up/Down scroll  r refresh  Esc settings  q quit"))
	current.writeStatus(&view)
	return view.String()
}

func (current model) metricRows() []string {
	paths := make([]string, 0, len(current.snapshot.Metrics))
	for path := range current.snapshot.Metrics {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	pathWidth := 48
	if current.width > 40 && current.width < 90 {
		pathWidth = current.width / 2
	}
	rows := make([]string, 0, len(paths))
	for _, path := range paths {
		envelope := current.snapshot.Metrics[path]
		value, err := json.Marshal(envelope.Value)
		if err != nil {
			value = fmt.Append(nil, envelope.Value)
		}
		unit := envelope.Unit
		line := fmt.Sprintf("%-*s  %s", pathWidth, truncate(path, pathWidth), string(value))
		if unit != "" {
			line += " " + unit
		}
		rows = append(rows, valueStyle.Render(line))
	}
	return rows
}

func (current *model) clampOffset() {
	maximum := max(0, len(current.snapshot.Metrics)-current.visibleMetricRows())
	current.offset = min(maximum, max(0, current.offset))
}

func (current model) visibleMetricRows() int {
	return max(4, current.height-8)
}

func (current model) featureEnabled(index int) bool {
	switch index {
	case 0:
		return current.configuration.Collectors.CPU
	case 1:
		return current.configuration.Collectors.Memory
	case 2:
		return current.configuration.Collectors.Thermal
	case 3:
		return current.configuration.Collectors.Storage
	case 4:
		return current.configuration.Collectors.Network
	case 5:
		return current.configuration.Collectors.GPU
	case 6:
		return current.configuration.Collectors.Display
	case 7:
		return current.configuration.Collectors.Hyprland
	case 8:
		return current.configuration.Collectors.Docker
	default:
		return false
	}
}

func (current *model) setFeature(index int, enabled bool) {
	switch index {
	case 0:
		current.configuration.Collectors.CPU = enabled
	case 1:
		current.configuration.Collectors.Memory = enabled
	case 2:
		current.configuration.Collectors.Thermal = enabled
	case 3:
		current.configuration.Collectors.Storage = enabled
	case 4:
		current.configuration.Collectors.Network = enabled
	case 5:
		current.configuration.Collectors.GPU = enabled
	case 6:
		current.configuration.Collectors.Display = enabled
	case 7:
		current.configuration.Collectors.Hyprland = enabled
	case 8:
		current.configuration.Collectors.Docker = enabled
	}
}

func (current model) writeStatus(view *strings.Builder) {
	if current.status == "" {
		return
	}
	view.WriteString("\n")
	if current.statusErr {
		view.WriteString(errorStyle.Render(current.status))
	} else {
		view.WriteString(enabledStyle.Render(current.status))
	}
}

func truncate(value string, width int) string {
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width <= 3 {
		return string(runes[:width])
	}
	return string(runes[:width-3]) + "..."
}
