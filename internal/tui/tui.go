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
	"pc-state-mqtt/internal/mqttclient"
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

type collectFeatureFunc func(context.Context, config.Config, string, time.Time) telemetry.Snapshot
type gatherFeatureFunc func(context.Context, config.Config, string, time.Time) systemstate.State

type publisher interface {
	PublishMetrics(context.Context, []telemetry.Metric) (int, error)
	PublishAvailability(context.Context, string) error
	Close(context.Context) error
}

type connectFunc func(context.Context, config.Config) (publisher, error)

type Options struct {
	Config     config.Config
	ConfigPath string
	Input      io.Reader
	Output     io.Writer
	Collect    collectFeatureFunc
	Gather     gatherFeatureFunc
	Connect    connectFunc
	Now        func() time.Time
}

type cadence uint8

const (
	sampleCadence cadence = iota
	discoveryCadence
	eventCadence
)

type feature struct {
	name        string
	description string
	implemented bool
	cadence     cadence
}

var features = []feature{
	{"CPU", "usage and per-core clocks", true, sampleCadence},
	{"Memory", "memory, cache, and swap", true, sampleCadence},
	{"Thermal", "temperature, fan, voltage, and power sensors", true, sampleCadence},
	{"Storage", "filesystems, block devices, and I/O", true, discoveryCadence},
	{"Network", "interfaces, addresses, and traffic", true, sampleCadence},
	{"GPU", "identity, utilization, clocks, and VRAM", false, sampleCadence},
	{"Display", "DRM connectors, EDID, and modes", false, eventCadence},
	{"Hyprland", "logical monitor and workspace state", false, eventCadence},
	{"Docker", "container state and resource usage", false, sampleCadence},
}

type featureRuntime struct {
	lastGathered time.Time
	lastSent     time.Time
	nextUpdate   time.Time
	metricCount  int
	err          string
	sendErr      string
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
	collect       collectFeatureFunc
	gather        gatherFeatureFunc
	connect       connectFunc
	now           func() time.Time
	publisher     publisher
	connecting    bool
	connectionErr string
	mode          viewMode
	cursor        int
	offset        int
	width         int
	height        int
	collecting    map[string]bool
	runtime       map[string]featureRuntime
	metrics       map[string][]telemetry.Metric
	snapshot      telemetry.Snapshot
	hasSnapshot   bool
	status        string
	statusErr     bool
}

type snapshotMsg struct {
	feature string
	state   systemstate.State
}

type connectedMsg struct {
	publisher publisher
	err       error
}

type publishedMsg struct {
	feature string
	at      time.Time
	count   int
	err     error
}

type refreshMsg struct {
	now time.Time
}

type saveMsg struct {
	err error
}

func Run(ctx context.Context, options Options) error {
	if options.Collect == nil {
		options.Collect = systemstate.CollectFeature
	}
	if options.Gather == nil {
		options.Gather = systemstate.GatherFeature
	}
	if options.Connect == nil {
		options.Connect = func(ctx context.Context, configuration config.Config) (publisher, error) {
			return mqttclient.Connect(ctx, configuration)
		}
	}
	program := tea.NewProgram(
		newModel(ctx, options),
		tea.WithAltScreen(),
		tea.WithContext(ctx),
		tea.WithInput(options.Input),
		tea.WithOutput(options.Output),
	)
	finalModel, err := program.Run()
	if result, ok := finalModel.(model); ok && result.publisher != nil {
		closeContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if closeErr := result.publisher.Close(closeContext); err == nil {
			err = closeErr
		}
	}
	return err
}

func newModel(ctx context.Context, options Options) model {
	collector := options.Collect
	if collector == nil {
		collector = systemstate.CollectFeature
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	gatherer := options.Gather
	if gatherer == nil {
		gatherer = func(ctx context.Context, configuration config.Config, featureName string, observedAt time.Time) systemstate.State {
			snapshot := collector(ctx, configuration, featureName, observedAt)
			return systemstate.State{Snapshot: snapshot}
		}
	}
	return model{
		ctx: ctx, configuration: options.Config, configPath: options.ConfigPath,
		collect: collector, gather: gatherer, connect: options.Connect, now: now, mode: settingsView,
		collecting: make(map[string]bool), runtime: make(map[string]featureRuntime),
		metrics:  make(map[string][]telemetry.Metric),
		snapshot: telemetry.NewSnapshot(options.Config.HostID, now(), nil, nil),
	}
}

func (current model) Init() tea.Cmd {
	if current.connect == nil {
		return nil
	}
	return current.connectCmd()
}

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
		current.mergeSnapshot(message.feature, message.state.Snapshot)
		current.hasSnapshot = true
		current.collecting[message.feature] = false
		runtime := current.runtime[message.feature]
		runtime.lastGathered = message.state.Snapshot.ObservedAt
		runtime.metricCount = len(message.state.Snapshot.Metrics)
		runtime.err = diagnosticText(message.state.Snapshot.Diagnostics)
		runtime.sendErr = ""
		if featureByName(message.feature).cadence != eventCadence {
			runtime.nextUpdate = message.state.Snapshot.ObservedAt.Add(current.intervalFor(featureByName(message.feature)))
		}
		current.runtime[message.feature] = runtime
		current.statusErr = false
		current.status = fmt.Sprintf("Gathered %s at %s", message.feature, message.state.Snapshot.ObservedAt.Local().Format("15:04:05"))
		current.clampOffset()
		current.metrics[message.feature] = message.state.Metrics
		if current.publisher != nil && len(message.state.Metrics) > 0 {
			return current, current.publishCmd(message.feature, message.state.Metrics)
		}
	case connectedMsg:
		current.connecting = false
		current.connectionErr = ""
		if message.err != nil {
			current.connectionErr = message.err.Error()
			current.statusErr = true
			current.status = "MQTT: " + message.err.Error()
			break
		}
		current.publisher = message.publisher
		current.statusErr = false
		current.status = "Connected to " + current.configuration.MQTT.BrokerURL
		commands := make([]tea.Cmd, 0, len(current.metrics))
		for featureName, metrics := range current.metrics {
			if len(metrics) > 0 {
				commands = append(commands, current.publishCmd(featureName, metrics))
			}
		}
		return current, tea.Batch(commands...)
	case publishedMsg:
		runtime := current.runtime[message.feature]
		if message.err != nil {
			runtime.sendErr = message.err.Error()
			current.statusErr = true
			current.status = fmt.Sprintf("Publish %s: %v", message.feature, message.err)
		} else {
			runtime.lastSent = message.at
			runtime.sendErr = ""
			current.statusErr = false
			current.status = fmt.Sprintf("Sent %d %s metrics at %s", message.count, message.feature, message.at.Local().Format("15:04:05"))
		}
		current.runtime[message.feature] = runtime
	case refreshMsg:
		current.snapshot.ObservedAt = message.now
		if current.mode == monitorView {
			current, command := current.startCollections(message.now, false)
			return current, tea.Batch(command, refreshAfter(time.Second))
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
		current.status = "Starting live watch..."
		current, command := current.startCollections(current.now(), true)
		return current, tea.Batch(command, refreshAfter(time.Second))
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
		current.status = "Refreshing enabled features..."
		return current.startCollections(current.now(), true)
	}
	return current, nil
}

func (current model) collectCmd(featureName string) tea.Cmd {
	ctx := current.ctx
	configuration := current.configuration
	gather := current.gather
	return func() tea.Msg {
		return snapshotMsg{feature: featureName, state: gather(ctx, configuration, featureName, current.now())}
	}
}

func (current model) connectCmd() tea.Cmd {
	ctx, configuration, connect := current.ctx, current.configuration, current.connect
	return func() tea.Msg {
		client, err := connect(ctx, configuration)
		return connectedMsg{publisher: client, err: err}
	}
}

func (current model) publishCmd(featureName string, metrics []telemetry.Metric) tea.Cmd {
	ctx, client, now := current.ctx, current.publisher, current.now
	return func() tea.Msg {
		count, err := client.PublishMetrics(ctx, metrics)
		return publishedMsg{feature: featureName, at: now(), count: count, err: err}
	}
}

func refreshAfter(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(now time.Time) tea.Msg { return refreshMsg{now: now} })
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
	view.WriteString(mutedStyle.Render("Up/Down select  Space toggle  s save  m live watch  q quit"))
	current.writeStatus(&view)
	return view.String()
}

func (current model) monitorView() string {
	var view strings.Builder
	view.WriteString(titleStyle.Render("pc-state-mqtt"))
	view.WriteString(mutedStyle.Render("  Live watch"))
	if current.anyCollecting() {
		view.WriteString(mutedStyle.Render("  refreshing"))
	}
	view.WriteString("\n")
	delivery := "MQTT delivery: connecting to " + current.configuration.MQTT.BrokerURL
	if current.publisher != nil {
		delivery = "MQTT delivery: connected to " + current.configuration.MQTT.BrokerURL + " (QoS 1 acknowledgements)"
	} else if current.connectionErr != "" {
		delivery = "MQTT delivery: " + current.connectionErr
	}
	view.WriteString(mutedStyle.Render(delivery))
	view.WriteString("\n\n")
	view.WriteString(mutedStyle.Render("Feature     State          Gathered             Sent       Next"))
	view.WriteByte('\n')
	for index, feature := range features {
		if !current.featureEnabled(index) {
			continue
		}
		view.WriteString(current.featureRuntimeRow(feature))
		view.WriteByte('\n')
	}
	view.WriteString("\n")
	view.WriteString(mutedStyle.Render("Data gathered / publish payload preview"))
	view.WriteByte('\n')
	if !current.hasSnapshot {
		view.WriteString("Waiting for the first feature update...\n")
	} else {
		rows := current.metricRows()
		end := min(len(rows), current.offset+current.visibleMetricRows())
		for _, row := range rows[current.offset:end] {
			view.WriteString(row)
			view.WriteByte('\n')
		}
	}
	view.WriteString("\n")
	view.WriteString(mutedStyle.Render("Up/Down scroll  r gather now  Esc settings  q quit"))
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
	enabledFeatures := 0
	for index := range features {
		if current.featureEnabled(index) {
			enabledFeatures++
		}
	}
	return max(4, current.height-enabledFeatures-12)
}

func (current model) startCollections(now time.Time, force bool) (model, tea.Cmd) {
	commands := make([]tea.Cmd, 0, len(features))
	for index, feature := range features {
		if !current.featureEnabled(index) {
			current.removeFeatureMetrics(feature.name)
			continue
		}
		if !feature.implemented || current.collecting[feature.name] {
			continue
		}
		runtime := current.runtime[feature.name]
		due := runtime.nextUpdate.IsZero() || !now.Before(runtime.nextUpdate)
		if feature.cadence == eventCadence {
			due = force && runtime.lastGathered.IsZero()
		}
		if force || due {
			current.collecting[feature.name] = true
			commands = append(commands, current.collectCmd(feature.name))
		}
	}
	return current, tea.Batch(commands...)
}

func (current *model) mergeSnapshot(featureName string, snapshot telemetry.Snapshot) {
	current.removeFeatureMetrics(featureName)
	for path, envelope := range snapshot.Metrics {
		current.snapshot.Metrics[path] = envelope
	}
	current.snapshot.ObservedAt = snapshot.ObservedAt
}

func (current *model) removeFeatureMetrics(featureName string) {
	prefix := strings.ToLower(featureName) + "/"
	for path := range current.snapshot.Metrics {
		if strings.HasPrefix(path, prefix) {
			delete(current.snapshot.Metrics, path)
		}
	}
}

func (current model) featureRuntimeRow(feature feature) string {
	runtime := current.runtime[feature.name]
	state := "waiting"
	gathered := "-"
	next := "due"
	if !feature.implemented {
		state = "planned"
		next = current.cadenceLabel(feature)
	} else if current.collecting[feature.name] {
		state = "gathering"
		next = "now"
	} else if runtime.err != "" {
		state = "error"
	}
	if !runtime.lastGathered.IsZero() {
		gathered = fmt.Sprintf("%s / %d", runtime.lastGathered.Local().Format("15:04:05"), runtime.metricCount)
	}
	sent := "not sent"
	if !runtime.lastSent.IsZero() {
		sent = runtime.lastSent.Local().Format("15:04:05")
	}
	if feature.implemented && feature.cadence == eventCadence {
		next = "event-driven"
	} else if feature.implemented && !runtime.nextUpdate.IsZero() && !current.collecting[feature.name] {
		next = formatCountdown(runtime.nextUpdate.Sub(current.snapshot.ObservedAt))
	}
	line := fmt.Sprintf("%-11s %-14s %-20s %-10s %s", feature.name, state, gathered, sent, next)
	rowError := strings.Join([]string{runtime.err, runtime.sendErr}, "; ")
	rowError = strings.Trim(rowError, "; ")
	if rowError != "" {
		line += "  " + rowError
		return errorStyle.Render(line)
	}
	if feature.implemented {
		return enabledStyle.Render(line)
	}
	return mutedStyle.Render(line)
}

func (current model) cadenceLabel(feature feature) string {
	if feature.cadence == eventCadence {
		return "event-driven"
	}
	return "every " + current.intervalFor(feature).String()
}

func (current model) intervalFor(feature feature) time.Duration {
	if feature.cadence == discoveryCadence {
		return current.configuration.DiscoveryInterval.Duration
	}
	return current.configuration.SampleInterval.Duration
}

func (current model) anyCollecting() bool {
	for _, collecting := range current.collecting {
		if collecting {
			return true
		}
	}
	return false
}

func featureByName(name string) feature {
	for _, feature := range features {
		if feature.name == name {
			return feature
		}
	}
	return feature{name: name, cadence: sampleCadence}
}

func diagnosticText(diagnostics []telemetry.Diagnostic) string {
	parts := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		parts = append(parts, diagnostic.Error)
	}
	return strings.Join(parts, "; ")
}

func formatCountdown(remaining time.Duration) string {
	if remaining <= 0 {
		return "due"
	}
	seconds := int64((remaining + time.Second - 1) / time.Second)
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	return fmt.Sprintf("%dm%02ds", seconds/60, seconds%60)
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
