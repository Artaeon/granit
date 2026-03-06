package tui

import (
	"sort"

	"github.com/charmbracelet/lipgloss"
)

// Theme holds every color role used throughout the UI.
type Theme struct {
	Name string
	// Accent colors
	Primary   lipgloss.Color // main accent (headings, borders)
	Secondary lipgloss.Color // links, h2
	Accent    lipgloss.Color // selection highlight, peach
	Warning   lipgloss.Color // yellow accents
	Success   lipgloss.Color // green, checkmarks
	Error     lipgloss.Color // red
	Info      lipgloss.Color // blue/cyan info
	// Text hierarchy
	Text    lipgloss.Color
	Subtext lipgloss.Color
	Dim     lipgloss.Color
	// Surface hierarchy
	Surface2 lipgloss.Color // line numbers
	Surface1 lipgloss.Color // unfocused borders
	Surface0 lipgloss.Color // highlights, code bg
	Base     lipgloss.Color // main bg
	Mantle   lipgloss.Color // status bar bg
	Crust    lipgloss.Color // help bar bg
}

// builtinThemes maps theme name to its definition.
var builtinThemes = map[string]Theme{
	"catppuccin-mocha": {
		Name:     "catppuccin-mocha",
		Primary:  lipgloss.Color("#CBA6F7"),
		Secondary: lipgloss.Color("#89B4FA"),
		Accent:   lipgloss.Color("#FAB387"),
		Warning:  lipgloss.Color("#F9E2AF"),
		Success:  lipgloss.Color("#A6E3A1"),
		Error:    lipgloss.Color("#F38BA8"),
		Info:     lipgloss.Color("#74C7EC"),
		Text:     lipgloss.Color("#CDD6F4"),
		Subtext:  lipgloss.Color("#BAC2DE"),
		Dim:      lipgloss.Color("#6C7086"),
		Surface2: lipgloss.Color("#585B70"),
		Surface1: lipgloss.Color("#45475A"),
		Surface0: lipgloss.Color("#313244"),
		Base:     lipgloss.Color("#1E1E2E"),
		Mantle:   lipgloss.Color("#181825"),
		Crust:    lipgloss.Color("#11111B"),
	},
	"catppuccin-latte": {
		Name:     "catppuccin-latte",
		Primary:  lipgloss.Color("#8839EF"),
		Secondary: lipgloss.Color("#1E66F5"),
		Accent:   lipgloss.Color("#FE640B"),
		Warning:  lipgloss.Color("#DF8E1D"),
		Success:  lipgloss.Color("#40A02B"),
		Error:    lipgloss.Color("#D20F39"),
		Info:     lipgloss.Color("#04A5E5"),
		Text:     lipgloss.Color("#4C4F69"),
		Subtext:  lipgloss.Color("#6C6F85"),
		Dim:      lipgloss.Color("#9CA0B0"),
		Surface2: lipgloss.Color("#ACB0BE"),
		Surface1: lipgloss.Color("#BCC0CC"),
		Surface0: lipgloss.Color("#CCD0DA"),
		Base:     lipgloss.Color("#EFF1F5"),
		Mantle:   lipgloss.Color("#E6E9EF"),
		Crust:    lipgloss.Color("#DCE0E8"),
	},
	"catppuccin-frappe": {
		Name:     "catppuccin-frappe",
		Primary:  lipgloss.Color("#CA9EE6"),
		Secondary: lipgloss.Color("#8CAAEE"),
		Accent:   lipgloss.Color("#EF9F76"),
		Warning:  lipgloss.Color("#E5C890"),
		Success:  lipgloss.Color("#A6D189"),
		Error:    lipgloss.Color("#E78284"),
		Info:     lipgloss.Color("#85C1DC"),
		Text:     lipgloss.Color("#C6D0F5"),
		Subtext:  lipgloss.Color("#B5BFE2"),
		Dim:      lipgloss.Color("#737994"),
		Surface2: lipgloss.Color("#626880"),
		Surface1: lipgloss.Color("#51576D"),
		Surface0: lipgloss.Color("#414559"),
		Base:     lipgloss.Color("#303446"),
		Mantle:   lipgloss.Color("#292C3C"),
		Crust:    lipgloss.Color("#232634"),
	},
	"catppuccin-macchiato": {
		Name:     "catppuccin-macchiato",
		Primary:  lipgloss.Color("#C6A0F6"),
		Secondary: lipgloss.Color("#8AADF4"),
		Accent:   lipgloss.Color("#F5A97F"),
		Warning:  lipgloss.Color("#EED49F"),
		Success:  lipgloss.Color("#A6DA95"),
		Error:    lipgloss.Color("#ED8796"),
		Info:     lipgloss.Color("#7DC4E4"),
		Text:     lipgloss.Color("#CAD3F5"),
		Subtext:  lipgloss.Color("#B8C0E0"),
		Dim:      lipgloss.Color("#6E738D"),
		Surface2: lipgloss.Color("#5B6078"),
		Surface1: lipgloss.Color("#494D64"),
		Surface0: lipgloss.Color("#363A4F"),
		Base:     lipgloss.Color("#24273A"),
		Mantle:   lipgloss.Color("#1E2030"),
		Crust:    lipgloss.Color("#181926"),
	},
	"tokyo-night": {
		Name:     "tokyo-night",
		Primary:  lipgloss.Color("#BB9AF7"),
		Secondary: lipgloss.Color("#7AA2F7"),
		Accent:   lipgloss.Color("#FF9E64"),
		Warning:  lipgloss.Color("#E0AF68"),
		Success:  lipgloss.Color("#9ECE6A"),
		Error:    lipgloss.Color("#F7768E"),
		Info:     lipgloss.Color("#2AC3DE"),
		Text:     lipgloss.Color("#C0CAF5"),
		Subtext:  lipgloss.Color("#A9B1D6"),
		Dim:      lipgloss.Color("#565F89"),
		Surface2: lipgloss.Color("#414868"),
		Surface1: lipgloss.Color("#3B4261"),
		Surface0: lipgloss.Color("#343B58"),
		Base:     lipgloss.Color("#1A1B26"),
		Mantle:   lipgloss.Color("#16161E"),
		Crust:    lipgloss.Color("#13131A"),
	},
	"gruvbox-dark": {
		Name:     "gruvbox-dark",
		Primary:  lipgloss.Color("#D3869B"),
		Secondary: lipgloss.Color("#83A598"),
		Accent:   lipgloss.Color("#FE8019"),
		Warning:  lipgloss.Color("#FABD2F"),
		Success:  lipgloss.Color("#B8BB26"),
		Error:    lipgloss.Color("#FB4934"),
		Info:     lipgloss.Color("#8EC07C"),
		Text:     lipgloss.Color("#EBDBB2"),
		Subtext:  lipgloss.Color("#D5C4A1"),
		Dim:      lipgloss.Color("#928374"),
		Surface2: lipgloss.Color("#665C54"),
		Surface1: lipgloss.Color("#504945"),
		Surface0: lipgloss.Color("#3C3836"),
		Base:     lipgloss.Color("#282828"),
		Mantle:   lipgloss.Color("#1D2021"),
		Crust:    lipgloss.Color("#141617"),
	},
	"nord": {
		Name:     "nord",
		Primary:  lipgloss.Color("#B48EAD"),
		Secondary: lipgloss.Color("#81A1C1"),
		Accent:   lipgloss.Color("#D08770"),
		Warning:  lipgloss.Color("#EBCB8B"),
		Success:  lipgloss.Color("#A3BE8C"),
		Error:    lipgloss.Color("#BF616A"),
		Info:     lipgloss.Color("#88C0D0"),
		Text:     lipgloss.Color("#ECEFF4"),
		Subtext:  lipgloss.Color("#D8DEE9"),
		Dim:      lipgloss.Color("#4C566A"),
		Surface2: lipgloss.Color("#434C5E"),
		Surface1: lipgloss.Color("#3B4252"),
		Surface0: lipgloss.Color("#2E3440"),
		Base:     lipgloss.Color("#242933"),
		Mantle:   lipgloss.Color("#1E222A"),
		Crust:    lipgloss.Color("#191D24"),
	},
	"dracula": {
		Name:     "dracula",
		Primary:  lipgloss.Color("#BD93F9"),
		Secondary: lipgloss.Color("#8BE9FD"),
		Accent:   lipgloss.Color("#FFB86C"),
		Warning:  lipgloss.Color("#F1FA8C"),
		Success:  lipgloss.Color("#50FA7B"),
		Error:    lipgloss.Color("#FF5555"),
		Info:     lipgloss.Color("#8BE9FD"),
		Text:     lipgloss.Color("#F8F8F2"),
		Subtext:  lipgloss.Color("#E2E2DC"),
		Dim:      lipgloss.Color("#6272A4"),
		Surface2: lipgloss.Color("#535669"),
		Surface1: lipgloss.Color("#44475A"),
		Surface0: lipgloss.Color("#383A4E"),
		Base:     lipgloss.Color("#282A36"),
		Mantle:   lipgloss.Color("#21222C"),
		Crust:    lipgloss.Color("#191A21"),
	},
	"solarized-dark": {
		Name:     "solarized-dark",
		Primary:  lipgloss.Color("#B58900"),
		Secondary: lipgloss.Color("#268BD2"),
		Accent:   lipgloss.Color("#CB4B16"),
		Warning:  lipgloss.Color("#B58900"),
		Success:  lipgloss.Color("#859900"),
		Error:    lipgloss.Color("#DC322F"),
		Info:     lipgloss.Color("#2AA198"),
		Text:     lipgloss.Color("#839496"),
		Subtext:  lipgloss.Color("#657B83"),
		Dim:      lipgloss.Color("#586E75"),
		Surface2: lipgloss.Color("#073642"),
		Surface1: lipgloss.Color("#073642"),
		Surface0: lipgloss.Color("#002B36"),
		Base:     lipgloss.Color("#002B36"),
		Mantle:   lipgloss.Color("#001E26"),
		Crust:    lipgloss.Color("#00141A"),
	},
	"solarized-light": {
		Name:     "solarized-light",
		Primary:  lipgloss.Color("#B58900"),
		Secondary: lipgloss.Color("#268BD2"),
		Accent:   lipgloss.Color("#CB4B16"),
		Warning:  lipgloss.Color("#B58900"),
		Success:  lipgloss.Color("#859900"),
		Error:    lipgloss.Color("#DC322F"),
		Info:     lipgloss.Color("#2AA198"),
		Text:     lipgloss.Color("#657B83"),
		Subtext:  lipgloss.Color("#839496"),
		Dim:      lipgloss.Color("#93A1A1"),
		Surface2: lipgloss.Color("#EEE8D5"),
		Surface1: lipgloss.Color("#EEE8D5"),
		Surface0: lipgloss.Color("#FDF6E3"),
		Base:     lipgloss.Color("#FDF6E3"),
		Mantle:   lipgloss.Color("#F5EFDC"),
		Crust:    lipgloss.Color("#EEE8D5"),
	},
	"rose-pine": {
		Name:     "rose-pine",
		Primary:  lipgloss.Color("#C4A7E7"),
		Secondary: lipgloss.Color("#9CCFD8"),
		Accent:   lipgloss.Color("#F6C177"),
		Warning:  lipgloss.Color("#F6C177"),
		Success:  lipgloss.Color("#31748F"),
		Error:    lipgloss.Color("#EB6F92"),
		Info:     lipgloss.Color("#9CCFD8"),
		Text:     lipgloss.Color("#E0DEF4"),
		Subtext:  lipgloss.Color("#908CAA"),
		Dim:      lipgloss.Color("#6E6A86"),
		Surface2: lipgloss.Color("#403D52"),
		Surface1: lipgloss.Color("#2A2837"),
		Surface0: lipgloss.Color("#26233A"),
		Base:     lipgloss.Color("#191724"),
		Mantle:   lipgloss.Color("#1F1D2E"),
		Crust:    lipgloss.Color("#16141F"),
	},
	"rose-pine-dawn": {
		Name:     "rose-pine-dawn",
		Primary:  lipgloss.Color("#907AA9"),
		Secondary: lipgloss.Color("#56949F"),
		Accent:   lipgloss.Color("#EA9D34"),
		Warning:  lipgloss.Color("#EA9D34"),
		Success:  lipgloss.Color("#286983"),
		Error:    lipgloss.Color("#B4637A"),
		Info:     lipgloss.Color("#56949F"),
		Text:     lipgloss.Color("#575279"),
		Subtext:  lipgloss.Color("#797593"),
		Dim:      lipgloss.Color("#9893A5"),
		Surface2: lipgloss.Color("#DFDAD9"),
		Surface1: lipgloss.Color("#F2E9E1"),
		Surface0: lipgloss.Color("#F4EDE8"),
		Base:     lipgloss.Color("#FAF4ED"),
		Mantle:   lipgloss.Color("#FFFAF3"),
		Crust:    lipgloss.Color("#F2E9E1"),
	},
	"everforest-dark": {
		Name:     "everforest-dark",
		Primary:  lipgloss.Color("#D699B6"),
		Secondary: lipgloss.Color("#7FBBB3"),
		Accent:   lipgloss.Color("#E69875"),
		Warning:  lipgloss.Color("#DBBC7F"),
		Success:  lipgloss.Color("#A7C080"),
		Error:    lipgloss.Color("#E67E80"),
		Info:     lipgloss.Color("#83C092"),
		Text:     lipgloss.Color("#D3C6AA"),
		Subtext:  lipgloss.Color("#9DA9A0"),
		Dim:      lipgloss.Color("#859289"),
		Surface2: lipgloss.Color("#543A48"),
		Surface1: lipgloss.Color("#374145"),
		Surface0: lipgloss.Color("#323D43"),
		Base:     lipgloss.Color("#2D353B"),
		Mantle:   lipgloss.Color("#272E33"),
		Crust:    lipgloss.Color("#232A2E"),
	},
	"kanagawa": {
		Name:     "kanagawa",
		Primary:  lipgloss.Color("#957FB8"),
		Secondary: lipgloss.Color("#7E9CD8"),
		Accent:   lipgloss.Color("#FFA066"),
		Warning:  lipgloss.Color("#DCA561"),
		Success:  lipgloss.Color("#98BB6C"),
		Error:    lipgloss.Color("#E82424"),
		Info:     lipgloss.Color("#7FB4CA"),
		Text:     lipgloss.Color("#DCD7BA"),
		Subtext:  lipgloss.Color("#C8C093"),
		Dim:      lipgloss.Color("#727169"),
		Surface2: lipgloss.Color("#363646"),
		Surface1: lipgloss.Color("#2A2A37"),
		Surface0: lipgloss.Color("#223249"),
		Base:     lipgloss.Color("#1F1F28"),
		Mantle:   lipgloss.Color("#1A1A22"),
		Crust:    lipgloss.Color("#16161D"),
	},
}

// ThemeNames returns the sorted list of available built-in theme names.
func ThemeNames() []string {
	names := make([]string, 0, len(builtinThemes))
	for name := range builtinThemes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetTheme returns a theme by name, or the default (catppuccin-mocha) if not found.
func GetTheme(name string) Theme {
	if t, ok := builtinThemes[name]; ok {
		return t
	}
	return builtinThemes["catppuccin-mocha"]
}

// ApplyTheme looks up the named theme and updates ALL package-level color
// variables and style variables in styles.go so every component picks up
// the new palette immediately.
func ApplyTheme(name string) {
	t := GetTheme(name)

	// ---- Update colour variables ----
	mauve = t.Primary
	blue = t.Secondary
	peach = t.Accent
	yellow = t.Warning
	green = t.Success
	red = t.Error
	sapphire = t.Info
	text = t.Text
	subtext1 = t.Subtext
	overlay0 = t.Dim
	surface2 = t.Surface2
	surface1 = t.Surface1
	surface0 = t.Surface0
	base = t.Base
	mantle = t.Mantle
	crust = t.Crust

	// Derived colour variables that some files reference directly.
	// Map them sensibly from the theme roles.
	rosewater = t.Accent    // warm accent fallback
	flamingo = t.Error      // close to red family
	pink = t.Primary        // close to primary/mauve
	maroon = t.Error        // red family
	teal = t.Info           // cool accent
	sky = t.Info            // cool accent
	lavender = t.Secondary  // blue family
	subtext0 = t.Subtext    // same bucket
	overlay1 = t.Dim        // dim family
	overlay2 = t.Dim        // dim family

	// ---- Rebuild every style variable ----

	// Panel styles
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

	FocusedBorderColor = mauve

	// Status bar
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

	// Help bar
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

	// Icons (pre-rendered strings)
	IconMd = lipgloss.NewStyle().Foreground(blue).Render("")
	IconFolder = lipgloss.NewStyle().Foreground(peach).Render("")
	IconDaily = lipgloss.NewStyle().Foreground(green).Render("")
	IconTag = lipgloss.NewStyle().Foreground(yellow).Render("")
}
