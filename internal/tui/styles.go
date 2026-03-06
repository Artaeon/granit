package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Catppuccin Mocha palette
	rosewater  = lipgloss.Color("#F5E0DC")
	flamingo   = lipgloss.Color("#F2CDCD")
	pink       = lipgloss.Color("#F5C2E7")
	mauve      = lipgloss.Color("#CBA6F7")
	red        = lipgloss.Color("#F38BA8")
	maroon     = lipgloss.Color("#EBA0AC")
	peach      = lipgloss.Color("#FAB387")
	yellow     = lipgloss.Color("#F9E2AF")
	green      = lipgloss.Color("#A6E3A1")
	teal       = lipgloss.Color("#94E2D5")
	sky        = lipgloss.Color("#89DCEB")
	sapphire   = lipgloss.Color("#74C7EC")
	blue       = lipgloss.Color("#89B4FA")
	lavender   = lipgloss.Color("#B4BEFE")
	text       = lipgloss.Color("#CDD6F4")
	subtext1   = lipgloss.Color("#BAC2DE")
	subtext0   = lipgloss.Color("#A6ADC8")
	overlay2   = lipgloss.Color("#9399B2")
	overlay1   = lipgloss.Color("#7F849C")
	overlay0   = lipgloss.Color("#6C7086")
	surface2   = lipgloss.Color("#585B70")
	surface1   = lipgloss.Color("#45475A")
	surface0   = lipgloss.Color("#313244")
	base       = lipgloss.Color("#1E1E2E")
	mantle     = lipgloss.Color("#181825")
	crust      = lipgloss.Color("#11111B")

	// Panel styles - unfocused
	PanelBorder = lipgloss.RoundedBorder()

	SidebarStyle = lipgloss.NewStyle().
			BorderStyle(PanelBorder).
			BorderForeground(surface1).
			Padding(0, 1)

	EditorStyle = lipgloss.NewStyle().
			BorderStyle(PanelBorder).
			BorderForeground(surface1).
			Padding(0, 1)

	BacklinksStyle = lipgloss.NewStyle().
			BorderStyle(PanelBorder).
			BorderForeground(surface1).
			Padding(0, 1)

	// Focused panel gets bright mauve border
	FocusedBorderColor = mauve

	// Status bar - two-tone vim-like
	StatusModeStyle = lipgloss.NewStyle().
			Background(mauve).
			Foreground(crust).
			Bold(true).
			Padding(0, 1)

	StatusFileStyle = lipgloss.NewStyle().
			Background(surface0).
			Foreground(text).
			Padding(0, 1)

	StatusInfoStyle = lipgloss.NewStyle().
			Background(surface1).
			Foreground(subtext0).
			Padding(0, 1)

	StatusBarBg = lipgloss.NewStyle().
			Background(mantle).
			Foreground(overlay0)

	// Help bar at the very bottom
	HelpBarStyle = lipgloss.NewStyle().
			Background(crust).
			Foreground(overlay0).
			Padding(0, 1)

	HelpKeyStyle = lipgloss.NewStyle().
			Background(crust).
			Foreground(lavender).
			Bold(true)

	HelpDescStyle = lipgloss.NewStyle().
			Background(crust).
			Foreground(overlay0)

	// Text styles
	TitleStyle = lipgloss.NewStyle().
			Foreground(mauve).
			Bold(true)

	H2Style = lipgloss.NewStyle().
		Foreground(blue).
		Bold(true)

	H3Style = lipgloss.NewStyle().
		Foreground(sapphire).
		Bold(true)

	SelectedStyle = lipgloss.NewStyle().
			Foreground(crust).
			Background(mauve).
			Bold(true).
			Padding(0, 1)

	SelectedItemStyle = lipgloss.NewStyle().
				Foreground(peach).
				Bold(true)

	NormalItemStyle = lipgloss.NewStyle().
			Foreground(text)

	DimStyle = lipgloss.NewStyle().
			Foreground(overlay0)

	LinkStyle = lipgloss.NewStyle().
			Foreground(blue).
			Underline(true)

	HeaderStyle = lipgloss.NewStyle().
			Foreground(mauve).
			Bold(true)

	// Markdown-specific
	BoldTextStyle = lipgloss.NewStyle().
			Foreground(text).
			Bold(true)

	ItalicTextStyle = lipgloss.NewStyle().
			Foreground(subtext1).
			Italic(true)

	CodeStyle = lipgloss.NewStyle().
			Foreground(green)

	CodeBlockStyle = lipgloss.NewStyle().
			Foreground(green).
			Background(surface0)

	FrontmatterStyle = lipgloss.NewStyle().
				Foreground(overlay1)

	ListMarkerStyle = lipgloss.NewStyle().
			Foreground(peach).
			Bold(true)

	CheckboxDone = lipgloss.NewStyle().
			Foreground(green)

	CheckboxTodo = lipgloss.NewStyle().
			Foreground(yellow)

	BlockquoteStyle = lipgloss.NewStyle().
			Foreground(overlay1).
			Italic(true)

	TagStyle = lipgloss.NewStyle().
			Foreground(crust).
			Background(blue).
			Padding(0, 1)

	// Line numbers
	LineNumStyle = lipgloss.NewStyle().
			Foreground(surface2).
			Width(5).
			Align(lipgloss.Right)

	ActiveLineNumStyle = lipgloss.NewStyle().
				Foreground(peach).
				Width(5).
				Align(lipgloss.Right).
				Bold(true)

	// Cursor
	CursorStyle = lipgloss.NewStyle().
			Background(text).
			Foreground(base)

	// Search
	SearchInputStyle = lipgloss.NewStyle().
				Foreground(text).
				Background(surface0).
				Padding(0, 1)

	SearchPromptStyle = lipgloss.NewStyle().
				Foreground(mauve).
				Bold(true)

	MatchHighlightStyle = lipgloss.NewStyle().
				Foreground(yellow).
				Bold(true)

	// File tree icons (using nerd font compatible unicode)
	IconMd     = lipgloss.NewStyle().Foreground(blue).Render("")
	IconFolder = lipgloss.NewStyle().Foreground(peach).Render("")
	IconDaily  = lipgloss.NewStyle().Foreground(green).Render("")
	IconTag    = lipgloss.NewStyle().Foreground(yellow).Render("")
)
