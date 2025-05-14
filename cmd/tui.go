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
	stepChooseAction     wizardStep = iota // Initial step: user selects the main action (replace, restore, clean, exit).
	stepEnterDir                         // Step: user inputs the target directory for the operation.
	stepEnterPattern                     // Step: user inputs the file pattern (glob) for the 'replace' action.
	stepEnterOldText                     // Step: user inputs the text to be searched for (for 'replace').
	stepEnterNewText                     // Step: user inputs the new text to replace the old text with.
	stepConfirmBackup                    // Step: user confirms whether to create .bak backup files (for 'replace').
	stepConfirmOperation                 // Step: user reviews a summary of the planned operation and confirms execution.
	stepShowResult                       // Step: displays the results or outcome of the completed operation.
	stepError                            // Step: displays an error message if an operation failed or an input was invalid.
)

// Action constants define the titles for user-selectable operations in the wizard.
const (
	actionReplace = "Replace Text in Files"
	actionRestore = "Restore Files from .bak"
	actionClean   = "Clean .bak Backup Files"
	actionExit    = "Exit"
)

// model holds the entire state of the TUI application.
// It's updated by the Update function in response to messages (events).
type model struct {
	step           wizardStep        // Current active step/screen in the wizard.
	actionList     list.Model        // Bubble component for selecting the main action.
	inputs         []textinput.Model // Slice to hold text input components (usually one active at a time).
	focusedInput   int               // Index of the currently focused text input (if multiple were on screen).
	backupChoice   list.Model        // Bubble component for Yes/No backup confirmation.
	spinner        spinner.Model     // Bubble component for displaying a loading spinner.
	isLoading      bool              // Flag indicating if a background operation is in progress.
	resultMessages []string          // Stores messages (e.g., list of modified files) to display after an operation.
	errorMessage   string            // Stores error messages to display to the user.
	quitting       bool              // Flag to signal that the application should quit.

	// Data collected from the wizard steps, to be used by core logic functions.
	selectedAction string // The action chosen by the user (e.g., "Replace Text").
	targetDir      string // The target directory for the file operation.
	filePattern    string // The file pattern (glob) for filtering files during replacement.
	oldText        string // The text to be searched for and replaced.
	newText        string // The new text that will replace the old text.
	shouldBackup   bool   // Flag indicating whether backup files should be created.

	width  int // Current terminal width, updated by WindowSizeMsg. Used for responsive layout.
	height int // Current terminal height, updated by WindowSizeMsg.
}

// operationResultMsg is a custom tea.Msg used to send results from a completed
// background operation (core logic) back to the TUI's Update function.
type operationResultMsg struct {
	messages  []string // Messages from the operation (e.g., list of modified files, summary).
	processed int      // Number of items processed (e.g., files scanned or matched).
}

// operationErrorMsg is a custom tea.Msg used to send an error from a failed
// background operation back to the TUI's Update function.
type operationErrorMsg struct{ err error }

// newWizardModel initializes and returns the starting state (model) of the TUI application.
func newWizardModel() model {
	// Setup for the main action selection list.
	actionItems := []list.Item{
		item{title: actionReplace, desc: "Search and replace text recursively in files."},
		item{title: actionRestore, desc: "Restore original files from their .bak backups."},
		item{title: actionClean, desc: "Delete all .bak backup files in a directory."},
		item{title: actionExit, desc: "Exit the application."},
	}
	// itemDelegate{} provides custom rendering for list items.
	// Default width/height are 0; they will be set by the first WindowSizeMsg.
	actionL := list.New(actionItems, itemDelegate{}, 0, 0)
	actionL.Title = "What would you like to do?"
	actionL.SetShowStatusBar(false)    // Hide the default status bar.
	actionL.SetFilteringEnabled(false) // No need for filtering in this simple list.
	actionL.Styles.Title = lipgloss.NewStyle().Bold(true).MarginBottom(1) // Style the list title.

	// Text Inputs will be initialized dynamically when needed for a specific step.
	// We use a slice to hold them, though typically only one is active.
	inputs := make([]textinput.Model, 1)

	// Setup for the backup confirmation list (Yes/No).
	backupItems := []list.Item{
		item{title: "Yes", desc: "Create .bak files before modification (recommended)."},
		item{title: "No", desc: "Do not create backups (use with caution)."},
	}
	backupL := list.New(backupItems, itemDelegate{}, 0, 0)
	backupL.Title = "Create .bak backups before replacing text?"
	backupL.SetShowStatusBar(false)
	backupL.SetFilteringEnabled(false)
	backupL.Styles.Title = lipgloss.NewStyle().Bold(true).MarginBottom(1)

	// Setup for the loading spinner.
	s := spinner.New()
	s.Spinner = spinner.Dot               // Choose a spinner style (e.g., Dot, Line, Points).
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205")) // Example spinner color.

	return model{
		step:         stepChooseAction, // Start at the action selection step.
		actionList:   actionL,
		inputs:       inputs,
		backupChoice: backupL,
		spinner:      s,
		// Other fields (isLoading, resultMessages, etc.) default to their zero values.
	}
}

// item struct implements the list.Item interface, used by the list.Model component.
// It holds the title and description for a list item.
type item struct {
	title, desc string
}

// Title returns the primary text of the list item.
func (i item) Title() string { return i.title }

// Description returns secondary, descriptive text for the list item.
func (i item) Description() string { return i.desc }

// FilterValue returns the string used by the list component for filtering items.
// Here, we use the title for filtering if it were enabled.
func (i item) FilterValue() string { return i.title }

// itemDelegate implements the list.ItemDelegate interface for custom rendering of items
// in the list.Model component.
type itemDelegate struct{}

// Height returns the height of a single rendered item line.
func (d itemDelegate) Height() int { return 1 } // Each item takes one line for title, description might add more implicitly via newline.

// Spacing returns the spacing between list items.
func (d itemDelegate) Spacing() int { return 0 } // No extra spacing between items.

// Update is called when a message is received by the list, allowing for custom item state updates.
// Not used in this simple delegate.
func (d itemDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }

// Render is called by the list component to draw a single list item.
// It writes the rendered output to the provided io.Writer.
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item) // Cast the list.Item back to our custom item type.
	if !ok {
		return // Should not happen if items are correctly added.
	}

	var strBuilder strings.Builder // Use a strings.Builder to construct the item's string representation.

	// Define styles for rendering list items.
	itemTitleStyle := lipgloss.NewStyle().PaddingLeft(2) // Style for non-selected item titles.
	selectedItemTitleStyle := lipgloss.NewStyle().PaddingLeft(0).Foreground(lipgloss.Color("62")).Bold(true) // Style for selected item titles.
	itemDescStyle := lipgloss.NewStyle().PaddingLeft(4).Faint(true) // Style for item descriptions.

	title := i.Title()
	if index == m.Index() { // Check if this is the currently selected item in the list.
		title = selectedItemTitleStyle.Render("> " + title) // Prepend ">" and apply selected style.
	} else {
		title = itemTitleStyle.Render(title) // Apply normal style.
	}
	strBuilder.WriteString(title + "\n") // Add the title to the builder.

	if i.Description() != "" { // If the item has a description, render and add it.
		desc := itemDescStyle.Render(i.Description())
		strBuilder.WriteString(desc + "\n")
	}

	// Write the fully constructed string for the item to the provided io.Writer.
	fmt.Fprint(w, strBuilder.String())
}

// Init is the first command that will be run when the Bubble Tea application starts.
// It can be used to initialize asynchronous operations or start timers.
func (m model) Init() tea.Cmd {
	return m.spinner.Tick // Start the spinner's ticking animation (it only shows when isLoading is true).
}

// Update is the core function that handles incoming messages (events) and updates the model's state.
// It returns the updated model and any command to be executed by Bubble Tea.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd    // A single command to be returned.
	var cmds []tea.Cmd // A slice to batch multiple commands.

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// This message is sent when the terminal window is resized.
		// Update model dimensions and resize Bubble components accordingly.
		m.width = msg.Width
		m.height = msg.Height
		// Adjust list heights, ensuring a minimum to prevent crashes.
		listHeight := msg.Height - 8 // Example adjustment, depends on other elements.
		if listHeight < 4 {
			listHeight = 4
		}
		m.actionList.SetSize(msg.Width-4, listHeight)   // -4 for some padding.
		m.backupChoice.SetSize(msg.Width-4, listHeight)
		// Text inputs can also be sized here if their width is dynamic.
		if len(m.inputs) > 0 && m.inputs[0].Focused() {
			m.inputs[0].Width = msg.Width - 10 // Adjust active input width.
			if m.inputs[0].Width < 20 { m.inputs[0].Width = 20 }
		}
		return m, nil // No command needed after resize.

	case tea.KeyMsg:
		// Handle global key bindings that apply across all steps.
		if msg.Type == tea.KeyCtrlC { // Ctrl+C always quits.
			m.quitting = true
			return m, tea.Quit
		}
		// 'Esc' key for navigation or exiting, if not loading.
		if msg.String() == "esc" && m.step > stepChooseAction && !m.isLoading {
			m.errorMessage = "" // Clear error message when navigating back.
			if m.step == stepShowResult || m.step == stepError {
				m.resetToMainMenu() // From result/error, Esc goes to main menu.
			} else {
				// Implement more granular "back" navigation based on current step and action.
				switch m.selectedAction {
				case actionReplace: // Back navigation specific to the 'Replace' flow.
					switch m.step {
					case stepEnterDir:          m.resetToMainMenu()
					case stepEnterPattern:      m.step = stepEnterDir; m.setupInput("Enter target directory:", m.targetDir)
					case stepEnterOldText:      m.step = stepEnterPattern; m.setupInput("Enter file pattern (e.g., *.txt):", m.filePattern)
					case stepEnterNewText:      m.step = stepEnterOldText; m.setupInput("Enter text to replace:", m.oldText)
					case stepConfirmBackup:     m.step = stepEnterNewText; m.setupInput("Enter new text:", m.newText)
					case stepConfirmOperation:  m.step = stepConfirmBackup // Go back to backup choice.
					}
				case actionRestore, actionClean: // Back navigation for 'Restore' and 'Clean'.
					switch m.step {
					case stepEnterDir:          m.resetToMainMenu()
					case stepConfirmOperation:  m.step = stepEnterDir; m.setupInput("Enter target directory:", m.targetDir)
					}
				default: // Fallback if selectedAction is not set (should not happen in a valid flow).
					m.resetToMainMenu()
				}
			}
			return m, nil // 'Esc' key press handled.
		}

		// Handle step-specific key presses. This is where most of the wizard flow logic resides.
		switch m.step {
		case stepChooseAction: // Handling input for the main action selection list.
			if msg.String() == "enter" {
				selectedItem, ok := m.actionList.SelectedItem().(item)
				if ok {
					m.selectedAction = selectedItem.title
					switch m.selectedAction {
					case actionReplace, actionRestore, actionClean:
						m.step = stepEnterDir // Proceed to directory input.
						m.setupInput("Enter target directory (e.g., ./project):", ".") // Setup text input.
					case actionExit:
						m.quitting = true
						return m, tea.Quit // Exit the application.
					}
				}
			}
			// Pass the message to the list component for its internal updates (navigation, etc.).
			m.actionList, cmd = m.actionList.Update(msg)
			cmds = append(cmds, cmd)

		case stepEnterDir: // Handling input for the target directory.
			if msg.String() == "enter" {
				m.targetDir = strings.TrimSpace(m.inputs[0].Value()) // Trim whitespace from input.
				if m.targetDir == "" { // Basic validation: directory cannot be empty.
					m.errorMessage = "Target directory cannot be empty."
					return m, nil // Stay on this step, display error.
				}
				// Validate if the entered path is an existing directory.
				info, err := os.Stat(m.targetDir)
				if os.IsNotExist(err) { // Path does not exist.
					m.errorMessage = fmt.Sprintf("Directory '%s' does not exist.", m.targetDir)
					return m, nil
				}
				if err != nil { // Other errors like permission denied.
					m.errorMessage = fmt.Sprintf("Error accessing directory '%s': %v", m.targetDir, err)
					return m, nil
				}
				if !info.IsDir() { // Path exists but is not a directory.
					m.errorMessage = fmt.Sprintf("Path '%s' is not a directory.", m.targetDir)
					return m, nil
				}
				m.errorMessage = "" // Clear any previous error if input is valid.
				// Proceed to the next step based on the selected action.
				switch m.selectedAction {
				case actionReplace:
					m.step = stepEnterPattern // For 'Replace', next is pattern input.
					m.setupInput("Enter file pattern (e.g., *.txt, default *):", "*")
				case actionRestore, actionClean:
					m.step = stepConfirmOperation // For 'Restore'/'Clean', go to confirmation.
				}
			} else { // Pass other key presses to the text input component.
				m.inputs[0], cmd = m.inputs[0].Update(msg)
				cmds = append(cmds, cmd)
			}

		case stepEnterPattern: // Handling input for file pattern (only for 'Replace' action).
			if msg.String() == "enter" {
				m.filePattern = strings.TrimSpace(m.inputs[0].Value())
				if m.filePattern == "" { m.filePattern = "*" } // Default to "*" if empty.

				// Validate the file pattern syntax using filepath.Match.
				// This is a basic check; complex valid patterns might still not match as expected by user.
				// "*" and "" are common valid wildcards, so allow them without specific match test.
				if _, err := filepath.Match(m.filePattern, "testfilename"); err != nil && m.filePattern != "*" && m.filePattern != "" {
					m.errorMessage = fmt.Sprintf("Invalid file pattern syntax: %v", err)
					return m, nil
				}
                m.errorMessage = "" // Clear error if pattern seems syntactically okay.

				m.step = stepEnterOldText
				m.setupInput("Enter text to replace:", "")
			} else {
				m.inputs[0], cmd = m.inputs[0].Update(msg)
				cmds = append(cmds, cmd)
			}
		
		case stepEnterOldText: // Handling input for 'Old Text' (only for 'Replace').
			if msg.String() == "enter" {
				m.oldText = m.inputs[0].Value() // Do not trim old/new text as spaces might be significant.
				// Validate that 'Old Text' is not empty if the action is 'Replace'.
				if m.oldText == "" && m.selectedAction == actionReplace { 
					m.errorMessage = "Text to replace cannot be empty for the 'Replace' action."
                    return m, nil
				}
                m.errorMessage = "" // Clear error.
				m.step = stepEnterNewText
				m.setupInput("Enter new text (leave empty to delete):", "")
			} else {
				m.inputs[0], cmd = m.inputs[0].Update(msg)
				cmds = append(cmds, cmd)
			}
		
		case stepEnterNewText: // Handling input for 'New Text' (only for 'Replace').
			if msg.String() == "enter" {
				m.newText = m.inputs[0].Value() // New text can be empty (implies deletion of OldText).
				m.step = stepConfirmBackup
				// No text input needed for backup choice; it's a list.
			} else {
				m.inputs[0], cmd = m.inputs[0].Update(msg)
				cmds = append(cmds, cmd)
			}
		
		case stepConfirmBackup: // Handling backup confirmation (Yes/No list, only for 'Replace').
			if msg.String() == "enter" {
				selectedItem, ok := m.backupChoice.SelectedItem().(item)
				if ok {
					m.shouldBackup = (selectedItem.title == "Yes")
					m.step = stepConfirmOperation // Proceed to final operation confirmation.
				}
			}
			m.backupChoice, cmd = m.backupChoice.Update(msg)
			cmds = append(cmds, cmd)

		case stepConfirmOperation: // Final confirmation step before executing the core logic.
			// User reviews the summary and presses Enter to proceed.
			if msg.String() == "enter" {
				m.isLoading = true         // Set loading state.
				m.resultMessages = nil     // Clear any previous results.
				m.errorMessage = ""        // Clear previous errors.
				// Return a command that will execute the core operation asynchronously.
				cmds = append(cmds, m.performOperationCmd())
			}

		case stepShowResult, stepError: // On result or error screen.
			// 'Enter' to go back to the main menu.
			if msg.Type == tea.KeyEnter {
				m.resetToMainMenu()
			}
		}

	// Handle custom messages from asynchronous operations (core logic functions).
	case operationResultMsg: // Message received when an operation completes successfully.
		m.isLoading = false // Clear loading state.
		m.resultMessages = msg.messages
		// Provide default messages if the operation didn't produce specific ones,
		// but did process some items.
		if len(m.resultMessages) == 0 && msg.processed > 0 {
			m.resultMessages = append(m.resultMessages, "No items matched the criteria or required modification.")
		} else if msg.processed == 0 && len(m.resultMessages) == 0 {
			// This case covers when no files were found or matched at all.
			m.resultMessages = append(m.resultMessages, "No items found or processed for the selected operation.")
		}
		m.step = stepShowResult // Move to the result display step.
		return m, nil           // No further command needed from this message.

	case operationErrorMsg: // Message received when an operation encounters an error.
		m.isLoading = false // Clear loading state.
		// Format the error message from the core operation for better display.
		m.errorMessage = fmt.Sprintf("Operation failed: %v", msg.err) 
		m.step = stepError               // Move to the error display step.
		return m, nil                    // No further command.

	// Handle spinner ticks for animation when isLoading is true.
	case spinner.TickMsg:
		var spCmd tea.Cmd
		if m.isLoading { // Only update spinner if loading.
			m.spinner, spCmd = m.spinner.Update(msg)
			cmds = append(cmds, spCmd)
		}
	} // End of switch msg.(type)
	// Batch all collected commands (from component updates, spinner, etc.) and return them.
	return m, tea.Batch(cmds...)
} // End of Update function

// setupInput configures a text input field for the current wizard step.
// It reinitializes m.inputs[0] for the new input.
func (m *model) setupInput(promptTextUnused string, placeholderValue string) { // promptTextUnused as prompt is in View now
	// Ensure m.inputs has at least one element. This should be true from initialization.
	if len(m.inputs) == 0 {
		m.inputs = make([]textinput.Model, 1) // Defensive allocation.
	}
	
	ti := textinput.New()
	// The prompt text is now typically rendered directly in the View() function for more layout control.
	ti.Placeholder = placeholderValue // Displayed when input is empty.
	ti.Focus()                        // Automatically focus the new input field.
	ti.CharLimit = 256                // Max characters allowed in the input.
	// Set input width based on terminal size, with padding.
	currentInputWidth := m.width - 10 
	if currentInputWidth < 20 { currentInputWidth = 20 } // Minimum width for usability.
	ti.Width = currentInputWidth
	
	m.inputs[0] = ti         // Assign the new text input to the model.
	m.focusedInput = 0       // If multiple inputs were used, this would track focus.
}

// resetToMainMenu resets the model's state to return to the initial action selection screen.
// This is used when an operation is complete or the user navigates back via 'Esc'.
func (m *model) resetToMainMenu() {
	m.step = stepChooseAction // Go back to the first step.
	// Clear all collected data from previous wizard flow.
	m.selectedAction = ""
	m.targetDir = ""
	m.filePattern = ""
	m.oldText = ""
	m.newText = ""
	m.shouldBackup = false
	m.errorMessage = ""      // Crucial: Clear any error messages when returning to main menu.
	m.resultMessages = nil   // Clear previous results.
	m.actionList.ResetFilter() // Clear any filter if it was enabled on the action list.
	m.actionList.Select(0)     // Select the first item in the action list by default.
	m.isLoading = false        // Ensure loading state is reset.
}

// performOperationCmd creates a tea.Cmd that, when executed by Bubble Tea,
// will run the appropriate core logic function (PerformReplacement, PerformRestore, etc.)
// based on the user's selections in the wizard. This happens asynchronously.
func (m model) performOperationCmd() tea.Cmd {
	return func() tea.Msg { // This anonymous function is the command.
		// It will be run by the Bubble Tea scheduler in a separate goroutine.
		switch m.selectedAction {
		case actionReplace:
			opts := ReplaceOptions{ // Gather options from the model.
				Dir:          m.targetDir,
				Pattern:      m.filePattern,
				OldText:      m.oldText,
				NewText:      m.newText,
				ShouldBackup: m.shouldBackup,
			}
			modified, processed, err := PerformReplacement(opts) // Call the core logic.
			if err != nil {
				return operationErrorMsg{err} // Send error message back to Update.
			}
			var messages []string
			for _, f := range modified { // Format success messages.
				messages = append(messages, "  - Modified: "+f)
			}
			return operationResultMsg{messages: messages, processed: processed} // Send result.
		
		case actionRestore:
			messages, err := PerformRestore(m.targetDir) // Call core logic.
			if err != nil {
				return operationErrorMsg{err}
			}
			// PerformRestore returns a slice of messages which might include a summary.
			// 'processed' count for restore/clean is approximated by len(messages) if no explicit count from core.
			return operationResultMsg{messages: messages, processed: len(messages)}
		
		case actionClean:
			messages, err := PerformClean(m.targetDir) // Call core logic.
			if err != nil {
				return operationErrorMsg{err}
			}
			return operationResultMsg{messages: messages, processed: len(messages)}
		}
		// This case should ideally not be reached if selectedAction is always valid.
		return operationErrorMsg{fmt.Errorf("internal error: unknown action selected: %s", m.selectedAction)}
	}
}

// View is called by Bubble Tea to render the current state of the TUI.
// It returns a string that represents the UI to be drawn to the terminal.
func (m model) View() string {
	if m.quitting { // If quitting flag is set, show a goodbye message.
		return "Exiting go-replace. Goodbye!\n"
	}

	var b strings.Builder // Use strings.Builder for efficient string concatenation to build the view.

	// Define some reusable styles using lipgloss for a nicer UI.
	// These can be customized extensively.
	titleStyle := lipgloss.NewStyle().Bold(true).MarginBottom(1).Foreground(lipgloss.Color("99")) // Example: Light purple
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).MarginBottom(1)              // Example: Red for errors
	resultHeaderStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")).MarginBottom(1) // Example: Green
	infoStyle := lipgloss.NewStyle().Faint(true).MarginTop(1) // For help text, etc.
	promptStyle := lipgloss.NewStyle().Bold(true)             // For input prompts.

	// If an operation is in progress, display the loading spinner.
	if m.isLoading {
		b.WriteString(fmt.Sprintf("%s Processing... please wait.\n", m.spinner.View()))
		return b.String() // Return early with just the spinner.
	}

	// Display error message if any. This is centralized to show errors consistently.
	if m.errorMessage != "" {
		b.WriteString(errorStyle.Render("Error: " + m.errorMessage) + "\n")
	}

	// Render the UI based on the current wizard step.
	switch m.step {
	case stepChooseAction:
		// The actionList component has its own View method.
		b.WriteString(m.actionList.View())
	
	case stepEnterDir:
		// Prompts are now rendered directly in the View for better control over layout and styling.
		b.WriteString(promptStyle.Render("Enter target directory (e.g., ./my_project):") + "\n")
		b.WriteString(m.inputs[0].View() + "\n") // Render the text input field.
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
		b.WriteString(promptStyle.Render("Enter new text (leave empty to delete):") + "\n")
		b.WriteString(m.inputs[0].View() + "\n")
		b.WriteString(infoStyle.Render("(Press Enter to confirm, Esc to go back)"))

	case stepConfirmBackup:
		// The backupChoice list component has its own View method.
		b.WriteString(m.backupChoice.View())

	case stepConfirmOperation:
		// Display a summary of the planned operation for final user confirmation.
		b.WriteString(titleStyle.Render("Confirm Operation Summary:") + "\n")
		b.WriteString(fmt.Sprintf("  Action: %s\n", m.selectedAction))
		b.WriteString(fmt.Sprintf("  Directory: %s\n", m.targetDir))
		if m.selectedAction == actionReplace { // Show replace-specific details.
			b.WriteString(fmt.Sprintf("  Pattern: %s\n", m.filePattern))
			b.WriteString(fmt.Sprintf("  Old Text: '%s'\n", m.oldText))
			b.WriteString(fmt.Sprintf("  New Text: '%s'\n", m.newText))
			b.WriteString(fmt.Sprintf("  Create Backups: %t\n", m.shouldBackup))
		}
		b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Render("Press Enter to proceed, Esc to go back."))

	case stepShowResult:
		// Display the results of the completed operation.
		b.WriteString(resultHeaderStyle.Render("Operation Complete:") + "\n")
		if len(m.resultMessages) > 0 {
			for _, resMsg := range m.resultMessages {
				b.WriteString(resMsg + "\n")
			}
		} else {
			// Fallback message if no specific results were provided by the core logic.
			b.WriteString("The operation finished, but no specific result messages were generated.\n")
		}
		b.WriteString("\n" + infoStyle.Render("(Press Enter to return to the main menu)"))

	case stepError:
		// The error message is already displayed globally at the top of the view.
		// This step just provides context for the "Press Enter" or "Esc" instruction.
		b.WriteString("\n" + infoStyle.Render("(Press Enter to return to the main menu)"))
	} // End of switch m.step

	// Return the fully constructed string representation of the UI.
	return b.String()
} // End of View function
