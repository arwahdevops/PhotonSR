package main

import (
	"fmt"
	"io"            // Required for io.Writer in list.ItemDelegate
	"os"            // Used for os.Stat to validate directories
	"path/filepath" // Used for filepath.Match to validate patterns
	"strings"       // Used for strings.Builder and other string manipulations

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss" // For advanced terminal styling
)

// --- TUI Model and Logic ---

// wizardStep defines the different stages or screens of the interactive TUI wizard.
type wizardStep int

const (
	stepChooseAction     wizardStep = iota // Initial step: user selects the main action.
	stepEnterDir                           // Step: user inputs the target directory.
	stepEnterPattern                       // Step: user inputs the file pattern (for 'replace').
	stepEnterOldText                       // Step: user inputs the text to be searched (for 'replace').
	stepEnterNewText                       // Step: user inputs the replacement text.
	stepConfirmBackup                      // Step: user confirms backup creation (for 'replace').
	stepConfirmOperation                   // Step: user reviews and confirms the operation.
	stepShowResult                         // Step: displays the outcome of the operation.
	stepError                              // Step: displays an error message.
)

// Action constants define the titles for user-selectable operations.
const (
	actionReplace = "Replace Text in Files"
	actionRestore = "Restore Files from .bak"
	actionClean   = "Clean .bak Backup Files"
	actionExit    = "Exit"
)

// model holds the entire state of the TUI application.
type model struct {
	step           wizardStep        // Current wizard step.
	actionList     list.Model        // List for choosing the main action.
	inputs         []textinput.Model // Text input components.
	focusedInput   int               // Index of the currently focused text input.
	backupChoice   list.Model        // List for Yes/No backup confirmation.
	spinner        spinner.Model     // Loading spinner.
	isLoading      bool              // True if a background operation is in progress.
	resultMessages []string          // Messages to display after an operation.
	errorMessage   string            // Error message to display.
	quitting       bool              // True if the application should quit.

	// Data collected from the wizard.
	selectedAction string // e.g., "Replace Text".
	targetDir      string // Target directory for the operation.
	filePattern    string // File pattern (glob) for replacement.
	oldText        string // Text to be replaced.
	newText        string // Replacement text.
	shouldBackup   bool   // Whether to create .bak files.

	width  int // Terminal width.
	height int // Terminal height.
}

// operationResultMsg is a tea.Msg for results from a background operation.
type operationResultMsg struct {
	detailMessages []string // Specific messages like "  - Modified: file.txt"
	itemsAffected  int      // Number of files modified, restored, or cleaned
	filesScanned   int      // For 'replace', total files scanned that matched pattern
}

// operationErrorMsg is a tea.Msg for an error from a background operation.
type operationErrorMsg struct{ err error }

// newWizardModel initializes the TUI model.
func newWizardModel() model {
	actionItems := []list.Item{
		item{title: actionReplace, desc: "Search and replace text recursively."},
		item{title: actionRestore, desc: "Restore original files from .bak backups."},
		item{title: actionClean, desc: "Delete all .bak backup files."},
		item{title: actionExit, desc: "Exit the application."},
	}
	actionL := list.New(actionItems, itemDelegate{}, 0, 0)
	actionL.Title = "What would you like to do?"
	actionL.SetShowStatusBar(false)
	actionL.SetFilteringEnabled(false)
	actionL.Styles.Title = lipgloss.NewStyle().Bold(true).MarginBottom(1)

	inputs := make([]textinput.Model, 1) // Typically one active input.

	backupItems := []list.Item{
		item{title: "Yes", desc: "Create .bak files (recommended)."},
		item{title: "No", desc: "Do not create backups (use with caution)."},
	}
	backupL := list.New(backupItems, itemDelegate{}, 0, 0)
	backupL.Title = "Create .bak backups before replacing text?"
	backupL.SetShowStatusBar(false)
	backupL.SetFilteringEnabled(false)
	backupL.Styles.Title = lipgloss.NewStyle().Bold(true).MarginBottom(1)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205")) // Pink spinner.

	return model{
		step:         stepChooseAction,
		actionList:   actionL,
		inputs:       inputs,
		backupChoice: backupL,
		spinner:      s,
	}
}

// item implements list.Item for use in list.Model.
type item struct {
	title, desc string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title } // Used for filtering if enabled.

// itemDelegate implements list.ItemDelegate for custom item rendering.
type itemDelegate struct{}

func (d itemDelegate) Height() int                               { return 1 } // Or 2 if desc is always shown
func (d itemDelegate) Spacing() int                              { return 0 }
func (d itemDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil } // Not used here.

func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}

	var strBuilder strings.Builder
	// Styles (can be pre-defined in model or globally for efficiency)
	itemTitleStyle := lipgloss.NewStyle().PaddingLeft(2)
	selectedItemTitleStyle := lipgloss.NewStyle().PaddingLeft(0).Foreground(lipgloss.Color("62")).Bold(true) // A nice green.
	itemDescStyle := lipgloss.NewStyle().PaddingLeft(4).Faint(true)                                          // Adjusted padding for alignment with "> "

	titleRender := itemTitleStyle.Render(i.Title())
	if index == m.Index() { // Is this item selected?
		titleRender = selectedItemTitleStyle.Render("> " + i.Title())
	}
	strBuilder.WriteString(titleRender)

	// Only render description if it exists and maybe only for selected/hovered or if Height allows
	// For simplicity here, render if exists. For a cleaner look with Height=1, desc could be omitted or shown elsewhere.
	// If Height() is 1, this will likely be truncated or overlap.
	// If you want multi-line items, delegate.Height() should be > 1.
	if i.Description() != "" {
		// Ensuring desc is on a new line if titles are single line
		strBuilder.WriteString("\n")
		descRender := itemDescStyle.Render(i.Description())
		strBuilder.WriteString(descRender)
	}
	// Ensure consistent line breaks for item height
	strBuilder.WriteString("\n")

	fmt.Fprint(w, strBuilder.String())
}

// Init is the first command run when the Bubble Tea application starts.
func (m model) Init() tea.Cmd {
	return m.spinner.Tick // Start spinner animation (only visible when isLoading).
}

// Update handles incoming messages and updates the model's state.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		listHeight := msg.Height - 8
		if listHeight < 4 {
			listHeight = 4
		}
		m.actionList.SetHeight(listHeight) // Use SetHeight for lists
		m.actionList.SetWidth(msg.Width - 4)
		m.backupChoice.SetHeight(listHeight)
		m.backupChoice.SetWidth(msg.Width - 4)

		if len(m.inputs) > 0 && m.inputs[0].Focused() {
			inputWidth := msg.Width - 10
			if inputWidth < 20 {
				inputWidth = 20
			}
			m.inputs[0].Width = inputWidth
		}
		return m, nil

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.quitting = true
			return m, tea.Quit
		}
		if msg.String() == "esc" && m.step > stepChooseAction && !m.isLoading {
			m.errorMessage = ""
			if m.step == stepShowResult || m.step == stepError {
				m.resetToMainMenu()
			} else {
				switch m.selectedAction {
				case actionReplace:
					switch m.step {
					case stepEnterDir:
						m.resetToMainMenu()
					case stepEnterPattern:
						m.step = stepEnterDir
						m.setupInputForCurrentStep()
					case stepEnterOldText:
						m.step = stepEnterPattern
						m.setupInputForCurrentStep()
					case stepEnterNewText:
						m.step = stepEnterOldText
						m.setupInputForCurrentStep()
					case stepConfirmBackup:
						m.step = stepEnterNewText
						m.setupInputForCurrentStep()
					case stepConfirmOperation:
						m.step = stepConfirmBackup
					}
				case actionRestore, actionClean:
					switch m.step {
					case stepEnterDir:
						m.resetToMainMenu()
					case stepConfirmOperation:
						m.step = stepEnterDir
						m.setupInputForCurrentStep()
					}
				default:
					m.resetToMainMenu()
				}
			}
			return m, nil
		}

		switch m.step {
		case stepChooseAction:
			if msg.String() == "enter" {
				selectedItem, ok := m.actionList.SelectedItem().(item)
				if ok {
					m.selectedAction = selectedItem.title
					switch m.selectedAction {
					case actionReplace, actionRestore, actionClean:
						m.step = stepEnterDir
						m.setupInputForCurrentStep()
					case actionExit:
						m.quitting = true
						return m, tea.Quit
					}
				}
			}
			m.actionList, cmd = m.actionList.Update(msg)
			cmds = append(cmds, cmd)

		case stepEnterDir:
			if msg.String() == "enter" {
				m.targetDir = strings.TrimSpace(m.inputs[0].Value())
				if m.targetDir == "" {
					m.targetDir = "."
				}
				m.errorMessage = ""
				info, err := os.Stat(m.targetDir)
				if os.IsNotExist(err) {
					m.errorMessage = fmt.Sprintf("Directory '%s' does not exist.", m.targetDir)
					return m, nil
				}
				if err != nil {
					m.errorMessage = fmt.Sprintf("Error accessing directory '%s': %v", m.targetDir, err)
					return m, nil
				}
				if !info.IsDir() {
					m.errorMessage = fmt.Sprintf("Path '%s' is not a directory.", m.targetDir)
					return m, nil
				}
				switch m.selectedAction {
				case actionReplace:
					m.step = stepEnterPattern
					m.setupInputForCurrentStep()
				case actionRestore, actionClean:
					m.step = stepConfirmOperation
				}
			} else {
				m.inputs[0], cmd = m.inputs[0].Update(msg)
				cmds = append(cmds, cmd)
			}

		case stepEnterPattern:
			if msg.String() == "enter" {
				m.filePattern = strings.TrimSpace(m.inputs[0].Value())
				if m.filePattern == "" {
					m.filePattern = "*"
				}
				m.errorMessage = ""
				if _, err := filepath.Match(m.filePattern, "testfilename"); err != nil && m.filePattern != "*" {
					m.errorMessage = fmt.Sprintf("Invalid file pattern syntax: %v", err)
					return m, nil
				}
				m.step = stepEnterOldText
				m.setupInputForCurrentStep()
			} else {
				m.inputs[0], cmd = m.inputs[0].Update(msg)
				cmds = append(cmds, cmd)
			}

		case stepEnterOldText:
			if msg.String() == "enter" {
				m.oldText = m.inputs[0].Value()
				m.errorMessage = ""
				if m.oldText == "" && m.selectedAction == actionReplace {
					m.errorMessage = "Text to replace cannot be empty for 'Replace' action."
					return m, nil
				}
				m.step = stepEnterNewText
				m.setupInputForCurrentStep()
			} else {
				m.inputs[0], cmd = m.inputs[0].Update(msg)
				cmds = append(cmds, cmd)
			}

		case stepEnterNewText:
			if msg.String() == "enter" {
				m.newText = m.inputs[0].Value()
				m.step = stepConfirmBackup
			} else {
				m.inputs[0], cmd = m.inputs[0].Update(msg)
				cmds = append(cmds, cmd)
			}

		case stepConfirmBackup:
			if msg.String() == "enter" {
				selectedItem, ok := m.backupChoice.SelectedItem().(item)
				if ok {
					m.shouldBackup = (selectedItem.title == "Yes")
					m.step = stepConfirmOperation
				}
			}
			m.backupChoice, cmd = m.backupChoice.Update(msg)
			cmds = append(cmds, cmd)

		case stepConfirmOperation:
			if msg.String() == "enter" {
				m.isLoading = true
				m.resultMessages = nil
				m.errorMessage = ""
				cmds = append(cmds, m.performOperationCmd())
			}

		case stepShowResult, stepError:
			if msg.Type == tea.KeyEnter {
				m.resetToMainMenu()
			}
		}

	case operationResultMsg:
		m.isLoading = false
		var finalMessages []string
		summary := ""

		switch m.selectedAction {
		case actionReplace:
			if msg.itemsAffected > 0 {
				summary = fmt.Sprintf("Successfully modified %d file(s).", msg.itemsAffected)
			} else if msg.filesScanned > 0 {
				summary = "Old text not found in any matching files, or files were already up-to-date."
			} else { // filesScanned == 0
				summary = "No files found matching the pattern in the specified directory."
			}
		case actionRestore:
			if msg.itemsAffected > 0 {
				summary = fmt.Sprintf("Successfully restored %d file(s).", msg.itemsAffected)
			} else {
				// Check if core logic provided a "no files found" message
				noFilesFoundMsgProvided := false
				for _, detailMsg := range msg.detailMessages {
					if strings.Contains(detailMsg, "No .bak files found to restore") {
						summary = detailMsg // Use the message from core logic
						noFilesFoundMsgProvided = true
						break
					}
				}
				if !noFilesFoundMsgProvided {
					summary = "No .bak files found to restore."
				}
			}
		case actionClean:
			if msg.itemsAffected > 0 {
				summary = fmt.Sprintf("Successfully cleaned %d backup file(s).", msg.itemsAffected)
			} else {
				noFilesFoundMsgProvided := false
				for _, detailMsg := range msg.detailMessages {
					if strings.Contains(detailMsg, "No .bak files found to clean") {
						summary = detailMsg
						noFilesFoundMsgProvided = true
						break
					}
				}
				if !noFilesFoundMsgProvided {
					summary = "No .bak files found to clean."
				}
			}
		}

		if summary != "" {
			finalMessages = append(finalMessages, summary)
		}
		if len(msg.detailMessages) > 0 && msg.itemsAffected > 0 { // Only add details if items were affected
			if summary != "" {
				finalMessages = append(finalMessages, "")
			} // Add a blank line for separation
			finalMessages = append(finalMessages, msg.detailMessages...)
		}

		if len(finalMessages) == 0 { // Fallback if no summary or details
			finalMessages = append(finalMessages, "Operation completed. No specific actions to report.")
		}

		m.resultMessages = finalMessages
		m.step = stepShowResult
		return m, nil

	case operationErrorMsg:
		m.isLoading = false
		m.errorMessage = fmt.Sprintf("Operation failed: %v", msg.err)
		m.step = stepError
		return m, nil

	case spinner.TickMsg:
		var spCmd tea.Cmd
		if m.isLoading {
			m.spinner, spCmd = m.spinner.Update(msg)
			cmds = append(cmds, spCmd)
		}
	}
	return m, tea.Batch(cmds...)
}

// setupInputForCurrentStep configures the text input field.
func (m *model) setupInputForCurrentStep() {
	if len(m.inputs) == 0 {
		m.inputs = make([]textinput.Model, 1)
	}
	ti := textinput.New()
	switch m.step {
	case stepEnterDir:
		ti.Placeholder = m.targetDir
		if ti.Placeholder == "" {
			ti.Placeholder = "."
		}
	case stepEnterPattern:
		ti.Placeholder = m.filePattern
		if ti.Placeholder == "" {
			ti.Placeholder = "*"
		}
	case stepEnterOldText:
		ti.Placeholder = m.oldText
	case stepEnterNewText:
		ti.Placeholder = m.newText
	}
	ti.Focus()
	ti.CharLimit = 256
	currentInputWidth := m.width - 10
	if currentInputWidth < 20 {
		currentInputWidth = 20
	}
	ti.Width = currentInputWidth
	m.inputs[0] = ti
	m.focusedInput = 0
}

// resetToMainMenu resets the model to the initial state.
func (m *model) resetToMainMenu() {
	m.step = stepChooseAction
	m.selectedAction = ""
	m.targetDir = ""
	m.filePattern = ""
	m.oldText = ""
	m.newText = ""
	m.shouldBackup = false
	m.errorMessage = ""
	m.resultMessages = nil
	m.actionList.ResetFilter()
	m.actionList.Select(0)
	m.isLoading = false
}

// performOperationCmd creates a tea.Cmd to run the core logic.
func (m model) performOperationCmd() tea.Cmd {
	return func() tea.Msg {
		switch m.selectedAction {
		case actionReplace:
			opts := ReplaceOptions{
				Dir: m.targetDir, Pattern: m.filePattern, OldText: m.oldText,
				NewText: m.newText, ShouldBackup: m.shouldBackup,
			}
			modifiedPaths, scanned, err := PerformReplacement(opts)
			if err != nil {
				return operationErrorMsg{err}
			}
			// PerformReplacement now returns detailed messages for "no files" or "no match" itself if needed,
			// but TUI constructs its own summary. So, detailMessages here are only for *actual modifications*.
			var dtlMsgs []string
			if len(modifiedPaths) > 0 { // Only populate if there were actual modifications
				for _, f := range modifiedPaths {
					dtlMsgs = append(dtlMsgs, "  - Modified: "+f)
				}
			}
			return operationResultMsg{detailMessages: dtlMsgs, itemsAffected: len(modifiedPaths), filesScanned: scanned}

		case actionRestore:
			dtlMsgs, restoredCount, err := PerformRestore(m.targetDir)
			if err != nil {
				return operationErrorMsg{err}
			}
			// Filter out the generic "No .bak files found..." from dtlMsgs if restoredCount is 0,
			// as the TUI summary will handle this. Keep only specific file messages.
			actualDetailMsgs := []string{}
			if restoredCount > 0 {
				for _, msg := range dtlMsgs {
					if strings.HasPrefix(strings.TrimSpace(msg), "- ") {
						actualDetailMsgs = append(actualDetailMsgs, msg)
					}
				}
			} else if len(dtlMsgs) == 1 && strings.Contains(dtlMsgs[0], "No .bak files found") {
				// If the only message is the "no files" summary from core, TUI will make its own.
				// So, pass empty detailMessages.
			} else {
				actualDetailMsgs = dtlMsgs // pass through if it's something else
			}
			return operationResultMsg{detailMessages: actualDetailMsgs, itemsAffected: restoredCount, filesScanned: restoredCount}

		case actionClean:
			dtlMsgs, cleanedCount, err := PerformClean(m.targetDir)
			if err != nil {
				return operationErrorMsg{err}
			}
			actualDetailMsgs := []string{}
			if cleanedCount > 0 {
				for _, msg := range dtlMsgs {
					if strings.HasPrefix(strings.TrimSpace(msg), "- ") {
						actualDetailMsgs = append(actualDetailMsgs, msg)
					}
				}
			} else if len(dtlMsgs) == 1 && strings.Contains(dtlMsgs[0], "No .bak files found") {
				// as above
			} else {
				actualDetailMsgs = dtlMsgs
			}
			return operationResultMsg{detailMessages: actualDetailMsgs, itemsAffected: cleanedCount, filesScanned: cleanedCount}
		}
		return operationErrorMsg{fmt.Errorf("internal error: unknown action: %s", m.selectedAction)}
	}
}

// View renders the TUI.
func (m model) View() string {
	if m.quitting {
		return "Exiting PhotonSR. Goodbye!\n"
	}

	var b strings.Builder
	// Styles
	titleStyle := lipgloss.NewStyle().Bold(true).MarginBottom(1).Foreground(lipgloss.Color("99"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).MarginBottom(1)
	resultHeaderStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")).MarginBottom(1)
	infoStyle := lipgloss.NewStyle().Faint(true).MarginTop(1)
	promptStyle := lipgloss.NewStyle().Bold(true)

	if m.isLoading {
		b.WriteString(fmt.Sprintf("%s Processing... please wait.\n", m.spinner.View()))
		return b.String()
	}

	if m.errorMessage != "" {
		b.WriteString(errorStyle.Render("Error: "+m.errorMessage) + "\n")
	}

	switch m.step {
	case stepChooseAction:
		b.WriteString(m.actionList.View())
	case stepEnterDir:
		b.WriteString(promptStyle.Render("Enter target directory (default: current directory '.'):") + "\n")
		b.WriteString(m.inputs[0].View() + "\n")
		b.WriteString(infoStyle.Render("(Press Enter to confirm, Esc to go back)"))
	case stepEnterPattern:
		b.WriteString(promptStyle.Render("Enter file pattern (e.g., *.txt, default *):") + "\n")
		b.WriteString(m.inputs[0].View() + "\n")
		b.WriteString(infoStyle.Render("(Press Enter to confirm, Esc to go back)"))
	case stepEnterOldText:
		b.WriteString(promptStyle.Render("Enter text to replace:") + "\n")
		b.WriteString(m.inputs[0].View() + "\n")
		b.WriteString(infoStyle.Render("(Press Enter to confirm, Esc to go back)"))
	case stepEnterNewText:
		b.WriteString(promptStyle.Render("Enter new text (leave empty to delete old text):") + "\n")
		b.WriteString(m.inputs[0].View() + "\n")
		b.WriteString(infoStyle.Render("(Press Enter to confirm, Esc to go back)"))
	case stepConfirmBackup:
		b.WriteString(m.backupChoice.View())
	case stepConfirmOperation:
		b.WriteString(titleStyle.Render("Confirm Operation Summary:") + "\n")
		b.WriteString(fmt.Sprintf("  Action: %s\n", m.selectedAction))
		b.WriteString(fmt.Sprintf("  Directory: %s\n", m.targetDir))
		if m.selectedAction == actionReplace {
			b.WriteString(fmt.Sprintf("  Pattern: %s\n", m.filePattern))
			b.WriteString(fmt.Sprintf("  Old Text: '%s'\n", m.oldText))
			b.WriteString(fmt.Sprintf("  New Text: '%s'\n", m.newText))
			b.WriteString(fmt.Sprintf("  Create Backups: %t\n", m.shouldBackup))
		}
		b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Render("Press Enter to proceed, Esc to go back."))
	case stepShowResult:
		b.WriteString(resultHeaderStyle.Render("Operation Complete:") + "\n")
		if len(m.resultMessages) > 0 {
			for _, resMsg := range m.resultMessages {
				b.WriteString(resMsg + "\n")
			}
		} else {
			b.WriteString("The operation finished, but no specific result messages were generated.\n")
		}
		b.WriteString("\n" + infoStyle.Render("(Press Enter to return to the main menu)"))
	case stepError:
		// Error message is displayed globally at the top.
		b.WriteString("\n" + infoStyle.Render("(Press Enter to return to the main menu or Esc to go back)"))
	}
	return b.String()
}
