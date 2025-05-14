package main

import (
	"fmt"
	"io"      // Required for io.Writer in list.ItemDelegate
	"os"      // Used for os.Stat to validate directories
	"path/filepath" // Used for filepath.Match to validate patterns
	"strings" // Used for strings.Builder and other string manipulations

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
	stepEnterDir                         // Step: user inputs the target directory.
	stepEnterPattern                     // Step: user inputs the file pattern (for 'replace').
	stepEnterOldText                     // Step: user inputs the text to be searched (for 'replace').
	stepEnterNewText                     // Step: user inputs the replacement text.
	stepConfirmBackup                    // Step: user confirms backup creation (for 'replace').
	stepConfirmOperation                 // Step: user reviews and confirms the operation.
	stepShowResult                       // Step: displays the outcome of the operation.
	stepError                            // Step: displays an error message.
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
	messages  []string // Messages from the operation (e.g., list of modified files).
	processed int      // Number of items processed (e.g., files scanned).
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

func (d itemDelegate) Height() int                               { return 1 }
func (d itemDelegate) Spacing() int                              { return 0 }
func (d itemDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil } // Not used here.

func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}

	var strBuilder strings.Builder
	itemTitleStyle := lipgloss.NewStyle().PaddingLeft(2)
	selectedItemTitleStyle := lipgloss.NewStyle().PaddingLeft(0).Foreground(lipgloss.Color("62")).Bold(true) // A nice green.
	itemDescStyle := lipgloss.NewStyle().PaddingLeft(4).Faint(true)

	title := i.Title()
	if index == m.Index() { // Is this item selected?
		title = selectedItemTitleStyle.Render("> " + title)
	} else {
		title = itemTitleStyle.Render(title)
	}
	strBuilder.WriteString(title + "\n")

	if i.Description() != "" {
		desc := itemDescStyle.Render(i.Description())
		strBuilder.WriteString(desc + "\n")
	}
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
		// Handle terminal resize.
		m.width = msg.Width
		m.height = msg.Height
		listHeight := msg.Height - 8 // Adjust based on other elements.
		if listHeight < 4 { listHeight = 4 } // Minimum height.
		m.actionList.SetSize(msg.Width-4, listHeight)
		m.backupChoice.SetSize(msg.Width-4, listHeight)
		if len(m.inputs) > 0 && m.inputs[0].Focused() {
			m.inputs[0].Width = msg.Width - 10
			if m.inputs[0].Width < 20 { m.inputs[0].Width = 20 }
		}
		return m, nil

	case tea.KeyMsg:
		// Global key bindings.
		if msg.Type == tea.KeyCtrlC {
			m.quitting = true
			return m, tea.Quit
		}
		if msg.String() == "esc" && m.step > stepChooseAction && !m.isLoading {
			m.errorMessage = "" // Clear error on back navigation.
			if m.step == stepShowResult || m.step == stepError {
				m.resetToMainMenu()
			} else {
				// Granular back navigation.
				switch m.selectedAction {
				case actionReplace:
					switch m.step {
					case stepEnterDir:         m.resetToMainMenu()
					case stepEnterPattern:     m.step = stepEnterDir; m.setupInputForCurrentStep()
					case stepEnterOldText:     m.step = stepEnterPattern; m.setupInputForCurrentStep()
					case stepEnterNewText:     m.step = stepEnterOldText; m.setupInputForCurrentStep()
					case stepConfirmBackup:    m.step = stepEnterNewText; m.setupInputForCurrentStep()
					case stepConfirmOperation: m.step = stepConfirmBackup
					}
				case actionRestore, actionClean:
					switch m.step {
					case stepEnterDir:         m.resetToMainMenu()
					case stepConfirmOperation: m.step = stepEnterDir; m.setupInputForCurrentStep()
					}
				default:
					m.resetToMainMenu()
				}
			}
			return m, nil
		}

		// Step-specific key handling.
		switch m.step {
		case stepChooseAction:
			if msg.String() == "enter" {
				selectedItem, ok := m.actionList.SelectedItem().(item)
				if ok {
					m.selectedAction = selectedItem.title
					switch m.selectedAction {
					case actionReplace, actionRestore, actionClean:
						m.step = stepEnterDir
						m.setupInputForCurrentStep() // Sets up input for directory.
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
				if m.targetDir == "" { // Default to current directory if input is empty.
					m.targetDir = "."
				}
				m.errorMessage = "" // Clear previous error.
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

				// Proceed to next step.
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
				if m.filePattern == "" { m.filePattern = "*" } // Default to "*"

				m.errorMessage = "" // Clear previous error.
				// Basic pattern syntax validation.
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
				m.oldText = m.inputs[0].Value() // Spaces might be significant.
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
				m.newText = m.inputs[0].Value() // New text can be empty.
				m.step = stepConfirmBackup
				// No text input needed for backup choice.
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
		m.resultMessages = msg.messages
		if len(m.resultMessages) == 0 && msg.processed > 0 {
			m.resultMessages = append(m.resultMessages, "No items matched the criteria or required modification.")
		} else if msg.processed == 0 && len(m.resultMessages) == 0 {
			m.resultMessages = append(m.resultMessages, "No items found or processed for the selected operation.")
		}
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

// setupInputForCurrentStep configures the text input field based on the current wizard step.
func (m *model) setupInputForCurrentStep() {
	if len(m.inputs) == 0 { m.inputs = make([]textinput.Model, 1) }

	ti := textinput.New()
	// Prompt text is rendered in View(). Placeholder gives a hint.
	switch m.step {
	case stepEnterDir:
		ti.Placeholder = m.targetDir // If navigating back, show previous value.
		if ti.Placeholder == "" { ti.Placeholder = "." }
	case stepEnterPattern:
		ti.Placeholder = m.filePattern
		if ti.Placeholder == "" { ti.Placeholder = "*" }
	case stepEnterOldText:
		ti.Placeholder = m.oldText
	case stepEnterNewText:
		ti.Placeholder = m.newText
	}

	ti.Focus()
	ti.CharLimit = 256
	currentInputWidth := m.width - 10
	if currentInputWidth < 20 { currentInputWidth = 20 }
	ti.Width = currentInputWidth

	m.inputs[0] = ti
	m.focusedInput = 0
}

// resetToMainMenu resets the model to the initial action selection screen.
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

// performOperationCmd creates a tea.Cmd to run the core logic function.
func (m model) performOperationCmd() tea.Cmd {
	return func() tea.Msg {
		switch m.selectedAction {
		case actionReplace:
			opts := ReplaceOptions{
				Dir: m.targetDir, Pattern: m.filePattern, OldText: m.oldText,
				NewText: m.newText, ShouldBackup: m.shouldBackup,
			}
			modified, processed, err := PerformReplacement(opts)
			if err != nil { return operationErrorMsg{err} }
			var messages []string
			for _, f := range modified {
				messages = append(messages, "  - Modified: "+f)
			}
			return operationResultMsg{messages: messages, processed: processed}

		case actionRestore:
			messages, err := PerformRestore(m.targetDir)
			if err != nil { return operationErrorMsg{err} }
			return operationResultMsg{messages: messages, processed: len(messages)} // Approx.

		case actionClean:
			messages, err := PerformClean(m.targetDir)
			if err != nil { return operationErrorMsg{err} }
			return operationResultMsg{messages: messages, processed: len(messages)} // Approx.
		}
		return operationErrorMsg{fmt.Errorf("internal error: unknown action: %s", m.selectedAction)}
	}
}

// View renders the current state of the TUI.
func (m model) View() string {
	if m.quitting {
		return "Exiting PhotonSR. Goodbye!\n"
	}

	var b strings.Builder
	titleStyle := lipgloss.NewStyle().Bold(true).MarginBottom(1).Foreground(lipgloss.Color("99")) // Light purple.
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).MarginBottom(1)             // Red for errors.
	resultHeaderStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")).MarginBottom(1) // Green.
	infoStyle := lipgloss.NewStyle().Faint(true).MarginTop(1) // For help text.
	promptStyle := lipgloss.NewStyle().Bold(true)            // For input prompts.

	if m.isLoading {
		b.WriteString(fmt.Sprintf("%s Processing... please wait.\n", m.spinner.View()))
		return b.String()
	}

	if m.errorMessage != "" {
		b.WriteString(errorStyle.Render("Error: " + m.errorMessage) + "\n")
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
