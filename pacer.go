// Pacer: the name simbolizes the waiting and progress animations
// and the flow of tasks in a terminal application.
package pacer

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"
)

// --- Theming and Customization ---

// Theme holds the ANSI codes for styling the output.
type Theme struct {
	ColorOK      string
	ColorFailed  string
	ColorRunning string
	StyleBold    string
	ColorReset   string
}

// DefaultTheme provides the standard color scheme.
var DefaultTheme = Theme{
	ColorOK:      "\033[32m", // Green
	ColorFailed:  "\033[31m", // Red
	ColorRunning: "\033[33m", // Yellow
	StyleBold:    "\033[1m",
	ColorReset:   "\033[0m",
}

// MonochromeTheme provides a theme with no color codes, suitable for simple terminals.
var MonochromeTheme = Theme{
	ColorOK:      "",
	ColorFailed:  "",
	ColorRunning: "",
	StyleBold:    "",
	ColorReset:   "",
}

// ANSI color and style codes for terminal output.
const (
	// ANSI escape code to clear the entire line.
	ClearLine = "\033[2K"
)

// Status represents the current state of a task.
type Status int

const (
	StatusRunning Status = iota
	StatusDone
	StatusFailed
)

// --- Task Options ---
// FormatFunc is a function that formats progress into a prefix string.
type FormatFunc func(current, total int) string

// Option is a function that configures a Task.
type Option func(*Task)

// WithoutTimer is an option to create a task without the elapsed time display.
func WithoutTimer() Option {
	return func(t *Task) {
		t.showTimer = false
	}
}

// WithProgress configures a task to track and display progress.
func WithProgress(total int, formatter FormatFunc) Option {
	return func(t *Task) {
		t.totalProgress = total
		t.progressFormatter = formatter
	}
}

// WithSpeed configures a task to calculate and display its speed.
func WithSpeed() Option {
	return func(t *Task) {
		t.isSpeedTask = true
	}
}

// WithProgressBar configures a task to display an ASCII progress bar.
func WithProgressBar(width int) Option {
	return func(t *Task) {
		t.isProgressBar = true
		t.progressBarWidth = width
	}
}

// WithSpinner adds a spinning character animation to the task.
func WithSpinner() Option {
	return func(t *Task) {
		t.isSpinnerTask = true
	}
}

// WithAutoClear removes the task from the display once it completes successfully.
func WithAutoClear() Option {
	return func(t *Task) {
		t.autoClear = true
	}
}

// WithSpinnerChars sets custom characters for the spinner animation.
func WithSpinnerChars(chars []string) Option {
	return func(t *Task) {
		if len(chars) > 0 {
			t.spinnerChars = chars
		}
	}
}

// WithProgressBarChars sets custom characters for the progress bar.
// 'filled' is the character for the completed part of the bar.
// 'head' is the character for the leading edge of the progress.
// 'empty' is the character for the remaining part of the bar.
func WithProgressBarChars(filled, head, empty string) Option {
	return func(t *Task) {
		t.progressBarFilled = filled
		t.progressBarHead = head
		t.progressBarEmpty = empty
	}
}

// WithTheme applies a custom theme to the task.
func WithTheme(theme Theme) Option {
	return func(t *Task) {
		t.theme = theme
	}
}

// WithQuietMode disables all animations and renders simple log output.
// Applying this to any task will put the entire manager in quiet mode.
func WithQuietMode() Option {
	return func(t *Task) {
		t.manager.isQuiet = true
	}
}

// WithFailOnSubtaskError causes a parent task to fail immediately if any of its subtasks fail.
func WithFailOnSubtaskError() Option {
	return func(t *Task) {
		t.failOnSubtaskError = true
	}
}

// --- Task ---
// Task represents a single unit of work being tracked.
type Task struct {
	message            string
	prefix             string
	status             Status
	err                error
	startTime          time.Time
	duration           time.Duration
	startIndex         int
	spinnerIndex       int
	showTimer          bool
	totalProgress      int
	currentProgress    int
	progressFormatter  FormatFunc
	isSpeedTask        bool
	isProgressBar      bool
	isSpinnerTask      bool
	autoClear          bool
	failOnSubtaskError bool
	progressBarWidth   int
	lastProgressTime   time.Time
	lastProgress       int
	speed              float64 // in units per second
	subtasks           []*Task
	parent             *Task
	manager            *Manager
	mu                 sync.Mutex
	// Customization fields
	spinnerChars      []string
	progressBarFilled string
	progressBarHead   string
	progressBarEmpty  string
	theme             Theme
}

// AddTask creates a new subtask and adds it to the current task.
func (t *Task) AddTask(message string, opts ...Option) *Task {
	return t.manager.addTask(message, t, opts...)
}

// Update sets a static prefix string for the task.
func (t *Task) Update(prefix string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.manager.isQuiet {
		return
	}
	t.prefix = prefix
}

// SetMessage updates the main display message of the task.
func (t *Task) SetMessage(message string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.manager.isQuiet {
		t.manager.Log(" -> %s", message)
	}
	t.message = message
}

// SetProgress updates the task's progress and its formatted prefix.
func (t *Task) SetProgress(current int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.manager.isQuiet {
		return
	}

	if t.isSpeedTask {
		now := time.Now()
		if !t.lastProgressTime.IsZero() {
			duration := now.Sub(t.lastProgressTime).Seconds()
			if duration > 0.1 { // Update speed only periodically to avoid flickering
				progressSinceLast := current - t.lastProgress
				if duration > 0 {
					t.speed = float64(progressSinceLast) / duration
				}
				t.lastProgressTime = now
				t.lastProgress = current
			}
		}
	}

	t.currentProgress = current
	if t.progressFormatter != nil {
		t.prefix = t.progressFormatter(current, t.totalProgress)
	}
}

// Success stops the task marking it as successful.
// This is a convenience method equivalent to calling Stop(nil).
func (t *Task) Success() {
	t.Stop(nil)
}

// Stop marks the task as completed, with an optional error.
func (t *Task) Stop(err error) {
	var shouldPropagate bool
	var parent *Task
	var parentFailOnSubtaskError bool
	var originalMessage string

	t.mu.Lock()
	// Only set the duration and status once.
	if t.status != StatusRunning {
		t.mu.Unlock()
		return
	}

	t.duration = time.Since(t.startTime)
	if err != nil {
		t.status = StatusFailed
		t.err = err
		shouldPropagate = true
		parent = t.parent
		if parent != nil {
			parentFailOnSubtaskError = parent.failOnSubtaskError
		}
		originalMessage = t.message
	} else {
		t.status = StatusDone
	}

	if t.manager.isQuiet {
		var statusStr string
		if err != nil {
			statusStr = fmt.Sprintf("[FAILED] %s: %v", t.message, err)
		} else {
			statusStr = fmt.Sprintf("[OK] %s", t.message)
		}
		fmt.Printf("%s (in %.2fs)\n", statusStr, t.duration.Seconds())
	}
	t.mu.Unlock()

	// Propagate failure upwards outside the lock to prevent deadlocks.
	if shouldPropagate && parent != nil && parentFailOnSubtaskError {
		parentErr := fmt.Errorf("subtask '%s' failed", originalMessage)
		parent.Stop(parentErr)
	}
}

// renderSelf generates the string representation of the task's own line, without subtasks.
func (t *Task) renderSelf() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	var timeStr, runningTimeStr string
	if t.showTimer {
		var elapsed time.Duration
		if t.status == StatusRunning {
			elapsed = time.Since(t.startTime)
		} else {
			elapsed = t.duration
		}
		minutes := int(elapsed.Minutes())
		seconds := int(elapsed.Seconds()) % 60
		timeStr = fmt.Sprintf(" [%02d:%02d]", minutes, seconds)
		runningTimeStr = fmt.Sprintf("[%02d:%02d] ", minutes, seconds)
	}

	switch t.status {
	case StatusDone:
		return fmt.Sprintf("%s[OK]%s%s %s", t.theme.ColorOK, t.theme.ColorReset, timeStr, t.message)
	case StatusFailed:
		errorLines := strings.Split(t.err.Error(), "\n")
		firstLine := fmt.Sprintf("%s[FAILED]%s%s %s: %s", t.theme.ColorFailed, t.theme.ColorReset, timeStr, t.message, errorLines[0])
		if len(errorLines) > 1 {
			indent := "        "
			if t.showTimer {
				indent = "            " // Adjust indent for the timer
			}
			var subsequentLines []string
			for _, line := range errorLines[1:] {
				subsequentLines = append(subsequentLines, fmt.Sprintf("%s%s", indent, line))
			}
			return firstLine + "\n" + strings.Join(subsequentLines, "\n")
		}
		return firstLine
	default: // StatusRunning
		var etaStr, speedStr, barStr, spinnerStr string
		if t.totalProgress > 0 && t.currentProgress > 0 && t.currentProgress < t.totalProgress {
			elapsed := time.Since(t.startTime)
			timePerUnit := float64(elapsed) / float64(t.currentProgress)
			remainingUnits := t.totalProgress - t.currentProgress
			eta := time.Duration(timePerUnit * float64(remainingUnits))
			etaMinutes := int(eta.Minutes())
			etaSeconds := int(eta.Seconds()) % 60
			etaStr = fmt.Sprintf(" (ETA %02dm%02ds)", etaMinutes, etaSeconds)
		}
		if t.isSpeedTask && t.speed > 0 {
			speedStr = fmt.Sprintf(" (%s)", FormatSpeed(t.speed))
		}
		// --- FIX 2: Added 't.totalProgress > 0' to prevent division by zero.
		if t.isProgressBar && t.totalProgress > 0 {
			percent := float64(t.currentProgress) / float64(t.totalProgress)
			filledWidth := int(percent * float64(t.progressBarWidth))
			if filledWidth > t.progressBarWidth {
				filledWidth = t.progressBarWidth
			}
			headWidth := len(t.progressBarHead)
			if filledWidth < headWidth {
				headWidth = filledWidth
			}
			filledPart := strings.Repeat(t.progressBarFilled, filledWidth-headWidth)
			headPart := t.progressBarHead[:headWidth]
			emptyPart := strings.Repeat(t.progressBarEmpty, t.progressBarWidth-filledWidth)
			bar := filledPart + headPart + emptyPart
			barStr = fmt.Sprintf("[%s] %3.f%% ", bar, percent*100)
			t.prefix = barStr // The bar replaces the standard prefix.
		} else if t.isSpinnerTask {
			spinnerStr = t.spinnerChars[t.spinnerIndex] + " "
			t.spinnerIndex = (t.spinnerIndex + 1) % len(t.spinnerChars)
		}

		runes := []rune(t.message)
		runeCount := len(runes)

		output := make([]string, runeCount)
		for i, r := range runes {
			output[i] = string(r)
		}

		// --- FIX 3: Added 'runeCount > 0' to prevent a divide-by-zero panic on empty messages.
		if runeCount > 0 {
			chunkSize := runeCount / 3
			if chunkSize < 1 {
				chunkSize = 1
			}

			for i := 0; i < chunkSize; i++ {
				idx := (t.startIndex + i) % runeCount
				output[idx] = fmt.Sprintf("%s%s%s%s", t.theme.StyleBold, t.theme.ColorRunning, string(runes[idx]), t.theme.ColorReset)
			}

			// Increment animation state for the next frame.
			t.startIndex = (t.startIndex + 1) % runeCount
		}

		return fmt.Sprintf("%s%s%s%s%s%s", runningTimeStr, spinnerStr, t.prefix, strings.Join(output, ""), etaStr, speedStr)
	}
}

// --- Manager ---
// Manager coordinates the rendering of multiple concurrent tasks.
type Manager struct {
	tasks        []*Task
	logHistory   []string
	mu           sync.Mutex
	ticker       *time.Ticker
	done         chan struct{}
	logChan      chan string
	wg           sync.WaitGroup
	linesDrawn   int
	shutdownOnce sync.Once
	isQuiet      bool
}

// NewManager creates a new Manager instance.
func NewManager() *Manager {
	return &Manager{
		tasks:      []*Task{},
		logHistory: []string{},
		ticker:     time.NewTicker(100 * time.Millisecond),
		done:       make(chan struct{}),
		logChan:    make(chan string, 100), // Buffered channel for logs
	}
}

// AddTask creates a new top-level task and adds it to the manager.
func (m *Manager) AddTask(message string, opts ...Option) *Task {
	return m.addTask(message, nil, opts...)
}

// addTask is the internal method for creating tasks, allowing for parents.
func (m *Manager) addTask(message string, parent *Task, opts ...Option) *Task {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Establish defaults.
	task := &Task{
		message:           message,
		status:            StatusRunning,
		startTime:         time.Now(),
		showTimer:         true,
		lastProgressTime:  time.Now(),
		manager:           m,
		spinnerChars:      []string{"|", "/", "-", "\\"},
		progressBarFilled: "█",
		progressBarHead:   "",
		progressBarEmpty:  "░",
		theme:             DefaultTheme,
	}

	// If a parent exists, inherit its customizable properties.
	if parent != nil {
		task.parent = parent
		task.spinnerChars = parent.spinnerChars
		task.theme = parent.theme
		task.progressBarFilled = parent.progressBarFilled
		task.progressBarHead = parent.progressBarHead
		task.progressBarEmpty = parent.progressBarEmpty
	}

	// Apply options specific to this task, potentially overriding inherited properties.
	for _, opt := range opts {
		opt(task)
	}

	if m.isQuiet {
		prefix := ""
		if parent != nil {
			prefix = "  "
		}
		fmt.Printf("%s[START] %s\n", prefix, task.message)
	}

	if parent != nil {
		parent.subtasks = append(parent.subtasks, task)
	} else {
		m.tasks = append(m.tasks, task)
	}
	return task
}

// Log sends a message to be printed above the task animations.
func (m *Manager) Log(format string, a ...interface{}) {
	if m.isQuiet {
		fmt.Printf(format+"\n", a...)
		return
	}
	// Prevent sending on closed channel
	defer func() {
		recover()
	}()
	m.logChan <- fmt.Sprintf(format, a...)
}

// OnInterrupt sets up a listener for OS signals and executes a callback
// before gracefully shutting down the manager and exiting.
func (m *Manager) OnInterrupt(callback func(), signals ...os.Signal) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, signals...)

	go func() {
		<-sigChan // Block until a signal is received
		m.Stop()
		if callback != nil {
			callback()
		}
		os.Exit(130) // Standard exit code for SIGINT
	}()
}

// Start begins the rendering loop for the manager.
func (m *Manager) Start() {
	if m.isQuiet {
		return
	}
	m.wg.Add(1)
	go m.renderLoop()
}

// Stop halts the rendering loop and performs one final render.
func (m *Manager) Stop() {
	if m.isQuiet {
		return
	}
	m.shutdownOnce.Do(func() {
		close(m.done)
		m.ticker.Stop()
		close(m.logChan)
	})

	m.wg.Wait() // Wait for the render loop to exit completely.

	// Drain any remaining logs that were in the channel when it was closed.
	for logMsg := range m.logChan {
		m.logHistory = append(m.logHistory, logMsg)
	}

	m.mu.Lock()
	// Perform one final cleanup before the final render.
	m.tasks = cleanTasks(m.tasks)
	m.render() // Perform one final render to show the final state.
	m.mu.Unlock()
}

// render handles drawing all logs and tasks to the terminal for a single frame.
// This function must be called within a lock.
func (m *Manager) render() {
	// Move cursor up to overwrite the previous render.
	if m.linesDrawn > 0 {
		fmt.Printf("\033[%dA", m.linesDrawn)
	}
	// Clear from the cursor to the end of the screen. This is crucial for removing
	// lines when the number of tasks decreases (e.g., due to auto-clearing).
	fmt.Print("\033[0J")

	lines := 0
	// Render logs
	for _, log := range m.logHistory {
		fmt.Printf("%s%s\n", ClearLine, log)
		lines++
	}

	// Render tasks
	for _, task := range m.tasks {
		m.renderTaskTree(task, "", &lines)
	}
	m.linesDrawn = lines
}

// renderTaskTree recursively renders a task and its subtasks with tree prefixes.
func (m *Manager) renderTaskTree(task *Task, prefix string, lines *int) {
	// Render the current task's line
	frame := task.renderSelf()
	fmt.Printf("%s%s%s\n", ClearLine, prefix, frame)
	*lines += 1 + strings.Count(frame, "\n")

	// Prepare prefix for children
	var childrenPrefix string
	if strings.HasSuffix(prefix, "├─ ") {
		childrenPrefix = strings.TrimSuffix(prefix, "├─ ") + "│  "
	} else if strings.HasSuffix(prefix, "└─ ") {
		childrenPrefix = strings.TrimSuffix(prefix, "└─ ") + "   "
	}

	// Render subtasks
	for i, subtask := range task.subtasks {
		var connector string
		if i < len(task.subtasks)-1 {
			connector = "├─ "
		} else {
			connector = "└─ "
		}
		m.renderTaskTree(subtask, childrenPrefix+connector, lines)
	}
}

// cleanTasks recursively removes tasks that are done and have autoClear set.
// It returns a new slice containing only the tasks that should be kept.
func cleanTasks(tasks []*Task) []*Task {
	// A new slice is allocated with a capacity based on the old slice,
	// which is a reasonable starting point for efficiency.
	newTasks := make([]*Task, 0, len(tasks))
	for _, task := range tasks {
		// A task is kept if it's not both 'Done' and marked for 'autoClear'.
		if !(task.status == StatusDone && task.autoClear) {
			// Recursively clean the subtasks of the current task.
			// This ensures the entire tree is pruned.
			task.subtasks = cleanTasks(task.subtasks)
			// Add the cleaned task to the new slice.
			newTasks = append(newTasks, task)
		}
	}
	return newTasks
}

// renderLoop is the main display function that runs in a goroutine.
func (m *Manager) renderLoop() {
	defer m.wg.Done()
	logChan := m.logChan // Use a local variable for the channel.
	for {
		select {
		case <-m.done:
			return
		case logMsg, ok := <-logChan:
			if ok {
				m.mu.Lock()
				m.logHistory = append(m.logHistory, logMsg)
				m.render()
				m.mu.Unlock()
			} else {
				// The channel has been closed. Set our local copy to nil to prevent
				// this case from being selected again, which would cause a busy-loop.
				logChan = nil
			}
		case <-m.ticker.C:
			m.mu.Lock()
			// Before rendering, prune the task list of any completed auto-clear tasks.
			m.tasks = cleanTasks(m.tasks)
			m.render()
			m.mu.Unlock()
		}
	}
}

// FormatSpeed converts bytes per second to a human-readable string.
func FormatSpeed(bytesPerSecond float64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case bytesPerSecond >= GB:
		return fmt.Sprintf("%.2f GB/s", bytesPerSecond/GB)
	case bytesPerSecond >= MB:
		return fmt.Sprintf("%.2f MB/s", bytesPerSecond/MB)
	case bytesPerSecond >= KB:
		return fmt.Sprintf("%.2f KB/s", bytesPerSecond/KB)
	default:
		return fmt.Sprintf("%.0f B/s", bytesPerSecond)
	}
}

// FormatBytesCount formats progress as "X MB / Y MB".
func FormatBytesCount(current, total int) string {
	format := func(b int) string {
		const (
			KB = 1024
			MB = 1024 * KB
			GB = 1024 * MB
		)
		val := float64(b)
		switch {
		case val >= GB:
			return fmt.Sprintf("%.2f GB", val/GB)
		case val >= MB:
			return fmt.Sprintf("%.2f MB", val/MB)
		case val >= KB:
			return fmt.Sprintf("%.2f KB", val/KB)
		default:
			return fmt.Sprintf("%d B", b)
		}
	}
	return fmt.Sprintf("[%s / %s] ", format(current), format(total))
}

// FormatPercent is a helper to create a percentage string for the prefix.
func FormatPercent(current, total int) string {
	// --- FIX 1: Added a check to prevent division by zero.
	if total == 0 {
		return "[  0%] "
	}
	percent := (float64(current) / float64(total)) * 100
	return fmt.Sprintf("[%3.f%%] ", percent)
}

// FormatCount is a helper to create a "M/N" count string for the prefix.
func FormatCount(current, total int) string {
	totalDigits := len(fmt.Sprintf("%d", total))
	formatString := fmt.Sprintf("[%%%dd/%d] ", totalDigits, total)
	return fmt.Sprintf(formatString, current)
}
