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
	"one-dark": {
		Name:     "one-dark",
		Primary:  lipgloss.Color("#C678DD"),
		Secondary: lipgloss.Color("#61AFEF"),
		Accent:   lipgloss.Color("#D19A66"),
		Warning:  lipgloss.Color("#E5C07B"),
		Success:  lipgloss.Color("#98C379"),
		Error:    lipgloss.Color("#E06C75"),
		Info:     lipgloss.Color("#56B6C2"),
		Text:     lipgloss.Color("#ABB2BF"),
		Subtext:  lipgloss.Color("#9DA5B4"),
		Dim:      lipgloss.Color("#5C6370"),
		Surface2: lipgloss.Color("#4B5263"),
		Surface1: lipgloss.Color("#3E4452"),
		Surface0: lipgloss.Color("#2C313C"),
		Base:     lipgloss.Color("#282C34"),
		Mantle:   lipgloss.Color("#21252B"),
		Crust:    lipgloss.Color("#1B1F27"),
	},
	"github-dark": {
		Name:     "github-dark",
		Primary:  lipgloss.Color("#D2A8FF"),
		Secondary: lipgloss.Color("#79C0FF"),
		Accent:   lipgloss.Color("#FFA657"),
		Warning:  lipgloss.Color("#E3B341"),
		Success:  lipgloss.Color("#7EE787"),
		Error:    lipgloss.Color("#FF7B72"),
		Info:     lipgloss.Color("#A5D6FF"),
		Text:     lipgloss.Color("#E6EDF3"),
		Subtext:  lipgloss.Color("#C9D1D9"),
		Dim:      lipgloss.Color("#8B949E"),
		Surface2: lipgloss.Color("#484F58"),
		Surface1: lipgloss.Color("#30363D"),
		Surface0: lipgloss.Color("#21262D"),
		Base:     lipgloss.Color("#0D1117"),
		Mantle:   lipgloss.Color("#090C10"),
		Crust:    lipgloss.Color("#060809"),
	},
	"github-light": {
		Name:     "github-light",
		Primary:  lipgloss.Color("#8250DF"),
		Secondary: lipgloss.Color("#0969DA"),
		Accent:   lipgloss.Color("#BF8700"),
		Warning:  lipgloss.Color("#9A6700"),
		Success:  lipgloss.Color("#1A7F37"),
		Error:    lipgloss.Color("#CF222E"),
		Info:     lipgloss.Color("#0550AE"),
		Text:     lipgloss.Color("#1F2328"),
		Subtext:  lipgloss.Color("#424A53"),
		Dim:      lipgloss.Color("#6E7781"),
		Surface2: lipgloss.Color("#AFB8C1"),
		Surface1: lipgloss.Color("#D0D7DE"),
		Surface0: lipgloss.Color("#E6EDF3"),
		Base:     lipgloss.Color("#FFFFFF"),
		Mantle:   lipgloss.Color("#F6F8FA"),
		Crust:    lipgloss.Color("#EAEEF2"),
	},
	"ayu-dark": {
		Name:     "ayu-dark",
		Primary:  lipgloss.Color("#D2A6FF"),
		Secondary: lipgloss.Color("#73D0FF"),
		Accent:   lipgloss.Color("#FFAD66"),
		Warning:  lipgloss.Color("#FFD173"),
		Success:  lipgloss.Color("#AAD94C"),
		Error:    lipgloss.Color("#F07178"),
		Info:     lipgloss.Color("#95E6CB"),
		Text:     lipgloss.Color("#BFBDB6"),
		Subtext:  lipgloss.Color("#ACB6BF"),
		Dim:      lipgloss.Color("#636A72"),
		Surface2: lipgloss.Color("#414851"),
		Surface1: lipgloss.Color("#2D323B"),
		Surface0: lipgloss.Color("#1A1F29"),
		Base:     lipgloss.Color("#0B0E14"),
		Mantle:   lipgloss.Color("#080A0F"),
		Crust:    lipgloss.Color("#05070A"),
	},
	"ayu-light": {
		Name:     "ayu-light",
		Primary:  lipgloss.Color("#A37ACC"),
		Secondary: lipgloss.Color("#399EE6"),
		Accent:   lipgloss.Color("#FA8D3E"),
		Warning:  lipgloss.Color("#F2AE49"),
		Success:  lipgloss.Color("#86B300"),
		Error:    lipgloss.Color("#F07171"),
		Info:     lipgloss.Color("#4CBF99"),
		Text:     lipgloss.Color("#5C6166"),
		Subtext:  lipgloss.Color("#787B80"),
		Dim:      lipgloss.Color("#9C9FA4"),
		Surface2: lipgloss.Color("#C4C4C4"),
		Surface1: lipgloss.Color("#D8D8D8"),
		Surface0: lipgloss.Color("#E8E8E8"),
		Base:     lipgloss.Color("#FCFCFC"),
		Mantle:   lipgloss.Color("#F3F3F3"),
		Crust:    lipgloss.Color("#E8E8E8"),
	},
	"palenight": {
		Name:     "palenight",
		Primary:  lipgloss.Color("#C792EA"),
		Secondary: lipgloss.Color("#82AAFF"),
		Accent:   lipgloss.Color("#F78C6C"),
		Warning:  lipgloss.Color("#FFCB6B"),
		Success:  lipgloss.Color("#C3E88D"),
		Error:    lipgloss.Color("#FF5370"),
		Info:     lipgloss.Color("#89DDFF"),
		Text:     lipgloss.Color("#A6ACCD"),
		Subtext:  lipgloss.Color("#959DCB"),
		Dim:      lipgloss.Color("#676E95"),
		Surface2: lipgloss.Color("#56597D"),
		Surface1: lipgloss.Color("#444267"),
		Surface0: lipgloss.Color("#373553"),
		Base:     lipgloss.Color("#292D3E"),
		Mantle:   lipgloss.Color("#232738"),
		Crust:    lipgloss.Color("#1B1E2E"),
	},
	"synthwave-84": {
		Name:     "synthwave-84",
		Primary:  lipgloss.Color("#F97E72"),
		Secondary: lipgloss.Color("#36F9F6"),
		Accent:   lipgloss.Color("#FF7EDB"),
		Warning:  lipgloss.Color("#FEDE5D"),
		Success:  lipgloss.Color("#72F1B8"),
		Error:    lipgloss.Color("#FE4450"),
		Info:     lipgloss.Color("#36F9F6"),
		Text:     lipgloss.Color("#FFFFFF"),
		Subtext:  lipgloss.Color("#E0D6F8"),
		Dim:      lipgloss.Color("#848BBD"),
		Surface2: lipgloss.Color("#495495"),
		Surface1: lipgloss.Color("#3B4178"),
		Surface0: lipgloss.Color("#2E2F56"),
		Base:     lipgloss.Color("#262335"),
		Mantle:   lipgloss.Color("#1E1A31"),
		Crust:    lipgloss.Color("#16122B"),
	},
	"nightfox": {
		Name:     "nightfox",
		Primary:  lipgloss.Color("#9D79D6"),
		Secondary: lipgloss.Color("#719CD6"),
		Accent:   lipgloss.Color("#F4A261"),
		Warning:  lipgloss.Color("#DFDFE0"),
		Success:  lipgloss.Color("#81B29A"),
		Error:    lipgloss.Color("#C94F6D"),
		Info:     lipgloss.Color("#63CDCF"),
		Text:     lipgloss.Color("#CDCECF"),
		Subtext:  lipgloss.Color("#AEAFB0"),
		Dim:      lipgloss.Color("#71839B"),
		Surface2: lipgloss.Color("#444A73"),
		Surface1: lipgloss.Color("#39506D"),
		Surface0: lipgloss.Color("#2B3B51"),
		Base:     lipgloss.Color("#192330"),
		Mantle:   lipgloss.Color("#131A24"),
		Crust:    lipgloss.Color("#0D1219"),
	},
	"vesper": {
		Name:     "vesper",
		Primary:  lipgloss.Color("#FFC799"),
		Secondary: lipgloss.Color("#8BA4B0"),
		Accent:   lipgloss.Color("#D19A66"),
		Warning:  lipgloss.Color("#FFD173"),
		Success:  lipgloss.Color("#A6CC70"),
		Error:    lipgloss.Color("#F07178"),
		Info:     lipgloss.Color("#7DCFFF"),
		Text:     lipgloss.Color("#B7B7B7"),
		Subtext:  lipgloss.Color("#8B8B8B"),
		Dim:      lipgloss.Color("#585858"),
		Surface2: lipgloss.Color("#343434"),
		Surface1: lipgloss.Color("#282828"),
		Surface0: lipgloss.Color("#1E1E1E"),
		Base:     lipgloss.Color("#101010"),
		Mantle:   lipgloss.Color("#0A0A0A"),
		Crust:    lipgloss.Color("#050505"),
	},
	"poimandres": {
		Name:     "poimandres",
		Primary:  lipgloss.Color("#5DE4C7"),
		Secondary: lipgloss.Color("#FCC5E9"),
		Accent:   lipgloss.Color("#89DDFF"),
		Warning:  lipgloss.Color("#FFFAC2"),
		Success:  lipgloss.Color("#5DE4C7"),
		Error:    lipgloss.Color("#D0679D"),
		Info:     lipgloss.Color("#ADD7FF"),
		Text:     lipgloss.Color("#E4F0FB"),
		Subtext:  lipgloss.Color("#A6ACCD"),
		Dim:      lipgloss.Color("#767C9D"),
		Surface2: lipgloss.Color("#3D4159"),
		Surface1: lipgloss.Color("#303340"),
		Surface0: lipgloss.Color("#252B37"),
		Base:     lipgloss.Color("#1B1E28"),
		Mantle:   lipgloss.Color("#171922"),
		Crust:    lipgloss.Color("#12141C"),
	},
	"moonlight": {
		Name:     "moonlight",
		Primary:  lipgloss.Color("#C099FF"),
		Secondary: lipgloss.Color("#86E1FC"),
		Accent:   lipgloss.Color("#FF966C"),
		Warning:  lipgloss.Color("#FFC777"),
		Success:  lipgloss.Color("#C3E88D"),
		Error:    lipgloss.Color("#FF757F"),
		Info:     lipgloss.Color("#77E0C6"),
		Text:     lipgloss.Color("#C8D3F5"),
		Subtext:  lipgloss.Color("#B4C2F0"),
		Dim:      lipgloss.Color("#636DA6"),
		Surface2: lipgloss.Color("#444A73"),
		Surface1: lipgloss.Color("#383E5C"),
		Surface0: lipgloss.Color("#2F3549"),
		Base:     lipgloss.Color("#222436"),
		Mantle:   lipgloss.Color("#1E2030"),
		Crust:    lipgloss.Color("#191A2A"),
	},
	"vitesse-dark": {
		Name:     "vitesse-dark",
		Primary:  lipgloss.Color("#4D9375"),
		Secondary: lipgloss.Color("#6394BF"),
		Accent:   lipgloss.Color("#D4976C"),
		Warning:  lipgloss.Color("#E6CC77"),
		Success:  lipgloss.Color("#4D9375"),
		Error:    lipgloss.Color("#CB7676"),
		Info:     lipgloss.Color("#5DA9D5"),
		Text:     lipgloss.Color("#DBD7CA"),
		Subtext:  lipgloss.Color("#B8B5A8"),
		Dim:      lipgloss.Color("#6B6B6B"),
		Surface2: lipgloss.Color("#393939"),
		Surface1: lipgloss.Color("#2C2C2C"),
		Surface0: lipgloss.Color("#1E1E1E"),
		Base:     lipgloss.Color("#121212"),
		Mantle:   lipgloss.Color("#0E0E0E"),
		Crust:    lipgloss.Color("#080808"),
	},
	"min-light": {
		Name:     "min-light",
		Primary:  lipgloss.Color("#4078F2"),
		Secondary: lipgloss.Color("#4078F2"),
		Accent:   lipgloss.Color("#4078F2"),
		Warning:  lipgloss.Color("#C18401"),
		Success:  lipgloss.Color("#50A14F"),
		Error:    lipgloss.Color("#E45649"),
		Info:     lipgloss.Color("#4078F2"),
		Text:     lipgloss.Color("#3B3B3B"),
		Subtext:  lipgloss.Color("#616161"),
		Dim:      lipgloss.Color("#9E9E9E"),
		Surface2: lipgloss.Color("#C8C8C8"),
		Surface1: lipgloss.Color("#D5D5D5"),
		Surface0: lipgloss.Color("#E8E8E8"),
		Base:     lipgloss.Color("#FAFAFA"),
		Mantle:   lipgloss.Color("#F0F0F0"),
		Crust:    lipgloss.Color("#E0E0E0"),
	},
	"oxocarbon": {
		Name:     "oxocarbon",
		Primary:  lipgloss.Color("#BE95FF"),
		Secondary: lipgloss.Color("#78A9FF"),
		Accent:   lipgloss.Color("#FF7EB6"),
		Warning:  lipgloss.Color("#EE5396"),
		Success:  lipgloss.Color("#42BE65"),
		Error:    lipgloss.Color("#FF6F61"),
		Info:     lipgloss.Color("#33B1FF"),
		Text:     lipgloss.Color("#F2F4F8"),
		Subtext:  lipgloss.Color("#DDE1E6"),
		Dim:      lipgloss.Color("#697077"),
		Surface2: lipgloss.Color("#474A4F"),
		Surface1: lipgloss.Color("#353535"),
		Surface0: lipgloss.Color("#262626"),
		Base:     lipgloss.Color("#161616"),
		Mantle:   lipgloss.Color("#0F0F0F"),
		Crust:    lipgloss.Color("#080808"),
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
