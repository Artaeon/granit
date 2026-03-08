package tui

import "github.com/charmbracelet/lipgloss"

// Color variables -- defaults are Catppuccin Mocha.
// ApplyTheme() in themes.go overwrites every one of these.
var (
	rosewater = lipgloss.Color("#F5E0DC")
	flamingo  = lipgloss.Color("#F2CDCD")
	pink      = lipgloss.Color("#F5C2E7")
	mauve     = lipgloss.Color("#CBA6F7")
	red       = lipgloss.Color("#F38BA8")
	maroon    = lipgloss.Color("#EBA0AC")
	peach     = lipgloss.Color("#FAB387")
	yellow    = lipgloss.Color("#F9E2AF")
	green     = lipgloss.Color("#A6E3A1")
	teal      = lipgloss.Color("#94E2D5")
	sky       = lipgloss.Color("#89DCEB")
	sapphire  = lipgloss.Color("#74C7EC")
	blue      = lipgloss.Color("#89B4FA")
	lavender  = lipgloss.Color("#B4BEFE")
	text      = lipgloss.Color("#CDD6F4")
	subtext1  = lipgloss.Color("#BAC2DE")
	subtext0  = lipgloss.Color("#A6ADC8")
	overlay2  = lipgloss.Color("#9399B2")
	overlay1  = lipgloss.Color("#7F849C")
	overlay0  = lipgloss.Color("#6C7086")
	surface2  = lipgloss.Color("#585B70")
	surface1  = lipgloss.Color("#45475A")
	surface0  = lipgloss.Color("#313244")
	base      = lipgloss.Color("#1E1E2E")
	mantle    = lipgloss.Color("#181825")
	crust     = lipgloss.Color("#11111B")
)

// Theme style properties — updated by ApplyTheme().
var (
	ThemeBorder    = "rounded"
	ThemeDensity   = "normal"
	ThemeAccentBar = "┃"
	ThemeSeparator = "─"
	ThemeLinkUL    = true
	WrapIndicator  = "↪"
)

// PanelPadding returns padding values based on current density.
func PanelPadding() (int, int) {
	switch ThemeDensity {
	case "compact":
		return 0, 0
	case "spacious":
		return 1, 2
	default:
		return 0, 1
	}
}

// ResolveBorder returns the lipgloss border for the current theme.
func ResolveBorder() lipgloss.Border {
	switch ThemeBorder {
	case "double":
		return lipgloss.DoubleBorder()
	case "thick":
		return lipgloss.ThickBorder()
	case "normal":
		return lipgloss.NormalBorder()
	case "hidden":
		return lipgloss.HiddenBorder()
	default:
		return lipgloss.RoundedBorder()
	}
}

// Style variables -- defaults built from the Catppuccin Mocha colours above.
// ApplyTheme() rebuilds every one of these when the user switches themes.
var (
	// Panel styles
	PanelBorder = lipgloss.RoundedBorder()

	SidebarStyle = lipgloss.NewStyle().
			BorderStyle(PanelBorder).
			BorderForeground(surface1).
			Background(base).
			Padding(0, 1)

	EditorStyle = lipgloss.NewStyle().
			BorderStyle(PanelBorder).
			BorderForeground(surface1).
			Background(base).
			Padding(0, 1)

	BacklinksStyle = lipgloss.NewStyle().
			BorderStyle(PanelBorder).
			BorderForeground(surface1).
			Background(base).
			Padding(0, 1)

	// Focused panel gets bright primary border
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
			Bold(true).
			Background(base)

	H2Style = lipgloss.NewStyle().
		Foreground(blue).
		Bold(true).
		Background(base)

	H3Style = lipgloss.NewStyle().
		Foreground(sapphire).
		Bold(true).
		Background(base)

	SelectedStyle = lipgloss.NewStyle().
			Foreground(crust).
			Background(mauve).
			Bold(true).
			Padding(0, 1)

	SelectedItemStyle = lipgloss.NewStyle().
				Foreground(peach).
				Bold(true)

	NormalItemStyle = lipgloss.NewStyle().
			Foreground(text).
			Background(base)

	DimStyle = lipgloss.NewStyle().
			Foreground(overlay0).
			Background(base)

	LinkStyle = lipgloss.NewStyle().
			Foreground(blue).
			Underline(true).
			Background(base)

	HeaderStyle = lipgloss.NewStyle().
			Foreground(mauve).
			Bold(true).
			Background(base)

	// Markdown-specific
	BoldTextStyle = lipgloss.NewStyle().
			Foreground(text).
			Bold(true).
			Background(base)

	ItalicTextStyle = lipgloss.NewStyle().
			Foreground(subtext1).
			Italic(true).
			Background(base)

	CodeStyle = lipgloss.NewStyle().
			Foreground(green)

	CodeBlockStyle = lipgloss.NewStyle().
			Foreground(green).
			Background(surface0)

	FrontmatterStyle = lipgloss.NewStyle().
				Foreground(overlay1).
				Background(base)

	ListMarkerStyle = lipgloss.NewStyle().
			Foreground(peach).
			Bold(true).
			Background(base)

	CheckboxDone = lipgloss.NewStyle().
			Foreground(green).
			Background(base)

	CheckboxTodo = lipgloss.NewStyle().
			Foreground(yellow).
			Background(base)

	BlockquoteStyle = lipgloss.NewStyle().
			Foreground(overlay1).
			Italic(true).
			Background(base)

	TagStyle = lipgloss.NewStyle().
			Foreground(crust).
			Background(blue).
			Padding(0, 1)

	// Line numbers
	LineNumStyle = lipgloss.NewStyle().
			Foreground(surface2).
			Width(5).
			Align(lipgloss.Right).
			Background(base)

	ActiveLineNumStyle = lipgloss.NewStyle().
				Foreground(peach).
				Width(5).
				Align(lipgloss.Right).
				Bold(true).
				Background(base)

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

	// File tree icons
	IconMd     = lipgloss.NewStyle().Foreground(blue).Render("◆")
	IconFolder = lipgloss.NewStyle().Foreground(peach).Render("▸")
	IconDaily  = lipgloss.NewStyle().Foreground(green).Render("◈")
	IconTag    = lipgloss.NewStyle().Foreground(yellow).Render("♯")
)

// Icon character variables — changed by ApplyIconTheme()
var (
	IconFileChar     = "◆"
	IconFolderChar   = "▸"
	IconDailyChar    = "◈"
	IconTagChar      = "♯"
	IconSearchChar   = "◉"
	IconBookmarkChar = "★"
	IconCanvasChar   = "◫"
	IconCalendarChar = "◇"
	IconEditChar     = "✎"
	IconViewChar     = "◈"
	IconLinkChar     = "⇄"
	IconGraphChar    = "◎"
	IconSettingsChar = "⚙"
	IconBotChar      = "⬡"
	IconTrashChar    = "▣"
	IconSaveChar     = "◇"
	IconHelpChar     = "?"
	IconNewChar      = "+"
	IconOutlineChar  = "≡"
)

func ApplyIconTheme(theme string) {
	switch theme {
	case "nerd":
		IconFileChar = "\uf0f6"
		IconFolderChar = "\uf07b"
		IconDailyChar = "\uf073"
		IconTagChar = "\uf02c"
		IconSearchChar = "\uf002"
		IconBookmarkChar = "\uf02e"
		IconCanvasChar = "\uf5fd"
		IconCalendarChar = "\uf073"
		IconEditChar = "\uf044"
		IconViewChar = "\uf06e"
		IconLinkChar = "\uf0c1"
		IconGraphChar = "\uf542"
		IconSettingsChar = "\uf013"
		IconBotChar = "\uf544"
		IconTrashChar = "\uf1f8"
		IconSaveChar = "\uf0c7"
		IconHelpChar = "\uf059"
		IconNewChar = "\uf067"
		IconOutlineChar = "\uf0ca"
	case "emoji":
		IconFileChar = "📄"
		IconFolderChar = "📁"
		IconDailyChar = "📅"
		IconTagChar = "🏷"
		IconSearchChar = "🔍"
		IconBookmarkChar = "⭐"
		IconCanvasChar = "🎨"
		IconCalendarChar = "📆"
		IconEditChar = "✏️"
		IconViewChar = "👁"
		IconLinkChar = "🔗"
		IconGraphChar = "🕸"
		IconSettingsChar = "⚙️"
		IconBotChar = "🤖"
		IconTrashChar = "🗑"
		IconSaveChar = "💾"
		IconHelpChar = "❓"
		IconNewChar = "➕"
		IconOutlineChar = "📋"
	case "ascii":
		IconFileChar = "~"
		IconFolderChar = ">"
		IconDailyChar = "@"
		IconTagChar = "#"
		IconSearchChar = "?"
		IconBookmarkChar = "*"
		IconCanvasChar = "+"
		IconCalendarChar = "="
		IconEditChar = "e"
		IconViewChar = "v"
		IconLinkChar = "<>"
		IconGraphChar = "o"
		IconSettingsChar = "%"
		IconBotChar = "b"
		IconTrashChar = "x"
		IconSaveChar = "s"
		IconHelpChar = "?"
		IconNewChar = "+"
		IconOutlineChar = "#"
	default: // "unicode"
		IconFileChar = "◆"
		IconFolderChar = "▸"
		IconDailyChar = "◈"
		IconTagChar = "♯"
		IconSearchChar = "◉"
		IconBookmarkChar = "★"
		IconCanvasChar = "◫"
		IconCalendarChar = "◇"
		IconEditChar = "✎"
		IconViewChar = "◈"
		IconLinkChar = "⇄"
		IconGraphChar = "◎"
		IconSettingsChar = "⚙"
		IconBotChar = "⬡"
		IconTrashChar = "▣"
		IconSaveChar = "◇"
		IconHelpChar = "?"
		IconNewChar = "+"
		IconOutlineChar = "≡"
	}

	// Rebuild the pre-styled icon strings
	IconMd = lipgloss.NewStyle().Foreground(blue).Render(IconFileChar)
	IconFolder = lipgloss.NewStyle().Foreground(peach).Render(IconFolderChar)
	IconDaily = lipgloss.NewStyle().Foreground(green).Render(IconDailyChar)
	IconTag = lipgloss.NewStyle().Foreground(yellow).Render(IconTagChar)
}
