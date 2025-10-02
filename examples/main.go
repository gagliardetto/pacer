package main

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/gagliardetto/pacer"
)

func main() {
	// --- Demo for Auto-failing parent ---
	fmt.Println("--- Auto-fail Parent Demo ---")
	failManager := pacer.NewManager()
	failManager.Start()
	// This parent task will fail when its subtask fails.
	failParent := failManager.AddTask("Deployment process", pacer.WithFailOnSubtaskError(), pacer.WithSpinner())
	time.Sleep(500 * time.Millisecond)
	// This subtask will succeed.
	subSuccess := failParent.AddTask("1. Pulling latest code")
	time.Sleep(1 * time.Second)
	subSuccess.Stop(nil)
	// This subtask will fail, triggering the parent to fail.
	subFail := failParent.AddTask("2. Running database migrations")
	time.Sleep(1 * time.Second)
	subFail.Stop(errors.New("migration script timeout"))
	// This task will never start because the parent will have already failed.
	subWontRun := failParent.AddTask("3. Deploying to production")
	time.Sleep(2 * time.Second) // Give time for the failure to propagate and be visible.
	subWontRun.Stop(nil)        // This will have no effect.
	failManager.Stop()

	// --- Demo for Quiet Mode ---
	fmt.Println("\n--- Quiet Mode Demo ---")
	quietManager := pacer.NewManager()
	// Adding WithQuietMode() to the first task sets the mode for the whole manager.
	qt1 := quietManager.AddTask("Top-level quiet task", pacer.WithQuietMode())
	quietManager.Start() // This will do nothing in quiet mode.
	time.Sleep(500 * time.Millisecond)
	qt2 := qt1.AddTask("Sub-task")
	time.Sleep(1 * time.Second)
	qt2.Stop(nil)
	qt1.Stop(nil)
	quietManager.Stop() // This will also do nothing.

	// --- Demo for Customization ---
	fmt.Println("\n--- Customization Demos ---")
	customManager := pacer.NewManager()
	customManager.Start()

	// Custom spinner
	spinnerTask := customManager.AddTask("Task with custom spinner", pacer.WithSpinner(), pacer.WithSpinnerChars([]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}))
	time.Sleep(2 * time.Second)
	spinnerTask.Stop(nil)

	// Custom progress bar
	barTask := customManager.AddTask("Task with custom progress bar", pacer.WithProgressBar(40), pacer.WithProgress(100, nil), pacer.WithProgressBarChars("=", ">", " "))
	for i := 0; i <= 100; i++ {
		barTask.SetProgress(i)
		time.Sleep(20 * time.Millisecond)
	}
	barTask.Stop(nil)

	// Custom theme (monochrome)
	themeTask := customManager.AddTask("Task with monochrome theme", pacer.WithTheme(pacer.MonochromeTheme), pacer.WithSpinner())
	time.Sleep(2 * time.Second)
	themeTask.Stop(errors.New("this is a themed error"))
	customManager.Stop()

	// --- Demo for Sequential Tasks ---
	fmt.Println("\n--- Sequential Demos ---")

	// Concurrent auto-clear demo
	managerConcurrentClear := pacer.NewManager()
	managerConcurrentClear.Start()

	filesToFetch := []string{
		"config.json", "user_data.csv", "styles.css", "app.js",
		"logo.svg", "api_spec.yaml", "README.md", "LICENSE",
	}
	totalFiles := len(filesToFetch)

	// Main task to track overall progress
	mainTask := managerConcurrentClear.AddTask(
		"Fetching project files",
		pacer.WithProgressBar(40),
		pacer.WithProgress(totalFiles, pacer.FormatCount),
	)

	var wg sync.WaitGroup
	completedFiles := 0
	var progressMu sync.Mutex

	for _, file := range filesToFetch {
		wg.Add(1)
		go func(filename string) {
			defer wg.Done()

			// Each subtask will auto-clear on completion
			subtask := mainTask.AddTask(
				fmt.Sprintf("Fetching %s", filename),
				pacer.WithSpinner(),
				pacer.WithAutoClear(),
			)

			// Simulate network latency
			time.Sleep(time.Duration(1000+rand.Intn(2000)) * time.Millisecond)
			subtask.Stop(nil)

			// Safely update the parent's progress
			progressMu.Lock()
			completedFiles++
			mainTask.SetProgress(completedFiles)
			progressMu.Unlock()
		}(file)
	}

	wg.Wait()
	mainTask.SetMessage("All files fetched successfully.")
	mainTask.Stop(nil)
	managerConcurrentClear.Stop()

	// Spinner task
	managerSpinner := pacer.NewManager()
	managerSpinner.Start()
	taskSpinner := managerSpinner.AddTask("Waiting for network...", pacer.WithSpinner())
	time.Sleep(2 * time.Second)
	taskSpinner.Stop(nil)
	managerSpinner.Stop()

	// Subtask demo
	managerSubtask := pacer.NewManager()
	managerSubtask.Start()
	parentTask := managerSubtask.AddTask("Running build process")
	parentTask.Update("-> ")
	time.Sleep(500 * time.Millisecond)
	subtask1 := parentTask.AddTask("Compiling assets")
	subtask1.Update("-> ")
	time.Sleep(1 * time.Second)
	subtask1.Stop(nil)
	subtask2 := parentTask.AddTask("Running tests")
	subtask2.Update("-> ")
	time.Sleep(1 * time.Second)
	subtask2.Stop(nil)
	parentTask.Stop(nil)
	managerSubtask.Stop()

	// Percentage task with ETA
	managerPercent := pacer.NewManager()
	managerPercent.Start()
	totalStepsPercent := 150
	taskPercent := managerPercent.AddTask("Processing data with percentage", pacer.WithProgress(totalStepsPercent, pacer.FormatPercent))
	for i := 0; i <= totalStepsPercent; i++ {
		taskPercent.SetProgress(i)
		time.Sleep(20 * time.Millisecond)
	}
	taskPercent.Stop(nil)
	managerPercent.Stop()

	// Progress bar task
	managerBar := pacer.NewManager()
	managerBar.Start()
	totalStepsBar := 200
	taskBar := managerBar.AddTask("Running build script", pacer.WithProgress(totalStepsBar, nil), pacer.WithProgressBar(40))
	for i := 0; i <= totalStepsBar; i++ {
		taskBar.SetProgress(i)
		time.Sleep(15 * time.Millisecond)
	}
	taskBar.Stop(nil)
	managerBar.Stop()

	// Count task with ETA
	managerCount := pacer.NewManager()
	managerCount.Start()
	totalStepsCount := 200
	taskCount := managerCount.AddTask("Downloading files with count", pacer.WithProgress(totalStepsCount, pacer.FormatCount))
	for i := 0; i <= totalStepsCount; i++ {
		taskCount.SetProgress(i)
		time.Sleep(15 * time.Millisecond)
	}
	taskCount.Stop(nil)
	managerCount.Stop()

	// Multi-download task with message updates
	managerMultiDownload := pacer.NewManager()
	managerMultiDownload.Start()
	filesToDownload := []string{"archive.zip", "document.pdf", "image.png", "video.mp4", "dataset.csv"}
	totalFiles = len(filesToDownload)
	taskMultiDownload := managerMultiDownload.AddTask("Starting multi-file download...", pacer.WithProgress(totalFiles, pacer.FormatCount))
	for i, filename := range filesToDownload {
		taskMultiDownload.SetMessage(fmt.Sprintf("Downloading %s", filename))
		taskMultiDownload.SetProgress(i + 1)
		time.Sleep(700 * time.Millisecond)
	}
	taskMultiDownload.SetMessage("Finished downloading all files.")
	taskMultiDownload.Stop(nil)
	managerMultiDownload.Stop()

	// Download task with speed and ETA
	managerSpeed := pacer.NewManager()
	managerSpeed.Start()
	totalDownloadSize := 50 * 1024 * 1024 // 50 MB
	taskSpeed := managerSpeed.AddTask("Downloading large_file.iso", pacer.WithProgress(totalDownloadSize, pacer.FormatBytesCount), pacer.WithSpeed())
	downloaded := 0
	for downloaded < totalDownloadSize {
		// Simulate variable download speed
		chunk := rand.Intn(3 * 1024 * 1024) // up to 3MB chunk
		downloaded += chunk
		if downloaded > totalDownloadSize {
			downloaded = totalDownloadSize
		}
		taskSpeed.SetProgress(downloaded)
		time.Sleep(200 * time.Millisecond)
	}
	taskSpeed.Stop(nil)
	managerSpeed.Stop()

	// Single-line failure task
	managerFail := pacer.NewManager()
	managerFail.Start()
	taskFail := managerFail.AddTask("Simulating a task that fails")
	taskFail.Update("-> ")
	time.Sleep(1 * time.Second)
	taskFail.Stop(errors.New("network connection timed out"))
	managerFail.Stop()

	// Multi-line failure task
	managerMultiFail := pacer.NewManager()
	managerMultiFail.Start()
	taskMultiFail := managerMultiFail.AddTask("Running complex deployment script")
	taskMultiFail.Update("-> ")
	time.Sleep(1 * time.Second)
	multiLineError := errors.New("deployment failed:\n  - Sub-task A timed out\n  - Sub-task B reported invalid configuration")
	taskMultiFail.Stop(multiLineError)
	managerMultiFail.Stop()

	// --- Demo for Complex Concurrent Build ---
	fmt.Println("\n--- Complex Concurrent Build Demo ---")
	buildManager := pacer.NewManager()
	buildManager.OnInterrupt(func() {
		fmt.Println("\nBuild interrupted. Exiting.")
	}, os.Interrupt)
	buildManager.Start()

	var buildWg sync.WaitGroup
	buildWg.Add(1)
	go func() {
		defer buildWg.Done()
		buildTask := buildManager.AddTask("Building project 'my-app'", pacer.WithSpinner())
		time.Sleep(500 * time.Millisecond)

		// Compile subtask
		compileTask := buildTask.AddTask("Compiling source code", pacer.WithProgressBar(30), pacer.WithProgress(100, nil))
		for i := 0; i <= 100; i++ {
			compileTask.SetProgress(i)
			time.Sleep(30 * time.Millisecond)
		}
		compileTask.Stop(nil)

		// Test subtask with its own subtasks
		testTask := buildTask.AddTask("Running tests", pacer.WithSpinner())
		time.Sleep(500 * time.Millisecond)

		unitTests := testTask.AddTask("Unit tests", pacer.WithSpinner())
		time.Sleep(2 * time.Second)
		unitTests.Stop(nil)

		integrationTests := testTask.AddTask("Integration tests", pacer.WithSpinner())
		time.Sleep(3 * time.Second)
		integrationTests.Stop(errors.New("database connection failed"))

		testTask.Stop(errors.New("one or more test suites failed"))

		// Package subtask (will not run if tests fail, but shown for structure)
		packageTask := buildTask.AddTask("Packaging artifacts", pacer.WithSpinner())
		time.Sleep(1 * time.Second)
		packageTask.Stop(nil)

		buildTask.Stop(errors.New("build failed"))
	}()

	buildWg.Wait()
	buildManager.Stop()

	fmt.Println("\nAll processes completed.")
}
