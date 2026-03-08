# Granit — Theme Reference

> Complete reference for all 35 built-in themes, the theme editor, and custom theme creation.

---

## Table of Contents

- [Overview](#overview)
- [Dark Themes (29)](#dark-themes-29)
- [Light Themes (6)](#light-themes-6)
- [Theme Editor](#theme-editor)
- [Custom Theme JSON Format](#custom-theme-json-format)
- [16 Color Roles Explained](#16-color-roles-explained)
- [Style Properties](#style-properties)
- [Applying Themes](#applying-themes)

---

## Overview

Granit ships with **35 built-in themes** (29 dark, 6 light) and supports user-created custom themes. Themes control all 16 color roles and 5 style properties throughout the entire UI.

- **Switch themes:** Settings (`Ctrl+,`) > "Theme"
- **Edit themes live:** Command palette > "Theme Editor"
- **Create themes:** Export from Theme Editor or write JSON manually
- **Custom theme location:** `~/.config/granit/themes/`

Theme changes are **instant** — no restart required.

---

## Dark Themes (29)

### Catppuccin Family

| Theme | Description | Primary | Secondary | Base | Style |
|-------|-------------|---------|-----------|------|-------|
| `catppuccin-mocha` | Warm, pastel dark **(default)** | `#CBA6F7` (Mauve) | `#89B4FA` (Blue) | `#1E1E2E` | Rounded borders, underlined links |
| `catppuccin-frappe` | Mid-tone Catppuccin | `#CA9EE6` | `#8CAAEE` | `#303446` | Rounded borders, underlined links |
| `catppuccin-macchiato` | Deep Catppuccin | `#C6A0F6` | `#8AADF4` | `#24273A` | Rounded borders, underlined links |

### Popular Editor Themes

| Theme | Description | Primary | Secondary | Base | Style |
|-------|-------------|---------|-----------|------|-------|
| `tokyo-night` | Inspired by Tokyo at night | `#BB9AF7` | `#7AA2F7` | `#1A1B26` | Rounded, thin accent bar |
| `gruvbox-dark` | Retro, earthy warm tones | `#D3869B` | `#83A598` | `#282828` | Normal borders, thick accent, no underline |
| `nord` | Arctic, cool blue palette | `#B48EAD` | `#81A1C1` | `#242933` | Rounded, spacious, thin accent |
| `dracula` | Classic dark with vivid accents | `#BD93F9` | `#8BE9FD` | `#282A36` | Thick borders, half-block accent |
| `solarized-dark` | Ethan Schoonover's dark palette | `#B58900` | `#268BD2` | `#002B36` | Normal borders, compact, no underline |
| `one-dark` | Atom's iconic dark theme | `#C678DD` | `#61AFEF` | `#282C34` | Rounded borders, underlined links |
| `github-dark` | GitHub dark mode | `#D2A8FF` | `#79C0FF` | `#0D1117` | Rounded borders, underlined links |
| `ayu-dark` | Minimal, deep dark | `#D2A6FF` | `#73D0FF` | `#0B0E14` | Rounded, thin accent bar |

### Aesthetic Themes

| Theme | Description | Primary | Secondary | Base | Style |
|-------|-------------|---------|-----------|------|-------|
| `rose-pine` | Muted, elegant dark | `#C4A7E7` | `#9CCFD8` | `#191724` | Rounded, spacious, dashed separator |
| `everforest-dark` | Nature-inspired greens | `#D699B6` | `#7FBBB3` | `#2D353B` | Rounded, spacious |
| `kanagawa` | Inspired by Hokusai's wave | `#957FB8` | `#7E9CD8` | `#1F1F28` | Normal borders, thin accent |
| `palenight` | Material Design dark variant | `#C792EA` | `#82AAFF` | `#292D3E` | Rounded borders |
| `synthwave-84` | Neon retro synthwave | `#F97E72` | `#36F9F6` | `#262335` | Double borders, spacious, thick separator |
| `nightfox` | Cool, refined dark | `#9D79D6` | `#719CD6` | `#192330` | Rounded borders |
| `vesper` | Warm amber on deep brown | `#FFC799` | `#8BA4B0` | `#101010` | Normal borders, compact, no underline |
| `poimandres` | Cool teal and pastels | `#5DE4C7` | `#FCC5E9` | `#1B1E28` | Rounded, thin accent, dashed separator |
| `moonlight` | Soft blue-purple moonlit | `#C099FF` | `#86E1FC` | `#222436` | Rounded borders |
| `vitesse-dark` | Minimal, modern green | `#4D9375` | `#6394BF` | `#121212` | Normal borders, compact, no underline |
| `oxocarbon` | IBM Carbon-inspired | `#BE95FF` | `#78A9FF` | `#161616` | Thick borders, half-block accent |

### Newest Additions (7)

| Theme | Description | Primary | Secondary | Base | Style |
|-------|-------------|---------|-----------|------|-------|
| `matrix` | Monochrome green-on-black terminal | `#00FF41` | `#008F11` | `#000000` | Normal borders, compact, no underline |
| `cobalt2` | Vibrant cobalt blue with gold | `#FFC600` | `#0088FF` | `#193549` | Thick borders, half-block accent |
| `monokai-pro` | Sublime Text's Monokai Pro | `#AB9DF2` | `#78DCE8` | `#2D2A2E` | Rounded borders |
| `horizon` | Warm sunset horizon colors | `#B877DB` | `#25B0BC` | `#1C1E26` | Rounded, thin accent bar |
| `zenburn` | Low-contrast, gentle on the eyes | `#F0DFAF` | `#8CD0D3` | `#303030` | Normal borders, spacious, no underline |
| `iceberg` | Cool blue-grey palette | `#A093C7` | `#84A0C6` | `#1E2132` | Rounded, thin accent bar |
| `amber` | Monochrome amber-on-black retro | `#FFB000` | `#FF8C00` | `#0A0800` | Normal borders, compact, no underline |

---

## Light Themes (6)

| Theme | Description | Primary | Secondary | Base | Style |
|-------|-------------|---------|-----------|------|-------|
| `catppuccin-latte` | Warm, pastel light Catppuccin | `#8839EF` | `#1E66F5` | `#EFF1F5` | Rounded borders, underlined links |
| `solarized-light` | Ethan Schoonover's light palette | `#B58900` | `#268BD2` | `#FDF6E3` | Normal borders, compact, no underline |
| `rose-pine-dawn` | Elegant, warm light | `#907AA9` | `#56949F` | `#FAF4ED` | Rounded, spacious, dashed separator |
| `github-light` | GitHub light mode | `#8250DF` | `#0969DA` | `#FFFFFF` | Rounded borders, underlined links |
| `ayu-light` | Clean, bright light | `#A37ACC` | `#399EE6` | `#FCFCFC` | Rounded borders, underlined links |
| `min-light` | Ultra-minimal bright | `#4078F2` | `#4078F2` | `#FAFAFA` | Hidden borders, compact, `>` accent, `·` separator |

---

## Theme Editor

The Theme Editor lets you live-edit all 16 color roles and preview changes in real time.

### Opening

- Command palette (`Ctrl+X`) > "Theme Editor"

### Usage

1. **Navigate** between color roles with `Up` / `Down`
2. **Edit** a color's hex value by pressing `Enter`
3. **Type** a new hex value (e.g., `#FF6B6B`)
4. **Preview** changes instantly — the entire UI updates as you type
5. **Save** the custom theme with a name
6. **Export** as a JSON file to `~/.config/granit/themes/`

### Controls

| Key | Action |
|-----|--------|
| `Up` / `Down` | Navigate between color roles |
| `Enter` | Edit the selected color value |
| `Esc` | Cancel editing / close editor |
| `s` | Save theme |
| `Tab` | Switch between color roles and style properties |

---

## Custom Theme JSON Format

Custom themes are JSON files stored in `~/.config/granit/themes/`. They follow the same structure as built-in themes.

### Full Template

```json
{
  "name": "my-custom-theme",
  "primary": "#FF6B6B",
  "secondary": "#4ECDC4",
  "accent": "#FFE66D",
  "warning": "#F7DC6F",
  "success": "#27AE60",
  "error": "#E74C3C",
  "info": "#3498DB",
  "text": "#ECF0F1",
  "subtext": "#BDC3C7",
  "dim": "#7F8C8D",
  "surface2": "#4A4A4A",
  "surface1": "#3A3A3A",
  "surface0": "#2A2A2A",
  "base": "#1A1A1A",
  "mantle": "#141414",
  "crust": "#0E0E0E",
  "border": "rounded",
  "density": "normal",
  "accent_bar": "┃",
  "separator": "─",
  "link_underline": true
}
```

### Creating a Custom Theme

1. Create the themes directory if it doesn't exist:
   ```bash
   mkdir -p ~/.config/granit/themes/
   ```

2. Create a JSON file with your theme definition:
   ```bash
   nano ~/.config/granit/themes/my-theme.json
   ```

3. Set your theme in Granit's settings:
   - Settings > "Theme" > select "my-custom-theme"
   - Or edit config: `"theme": "my-custom-theme"`

### Overriding Built-In Themes

If a custom theme has the same name as a built-in theme, the custom theme takes priority. This lets you modify a built-in theme without losing the original.

---

## 16 Color Roles Explained

### Accent Colors (7)

| Role | Purpose | Where It Appears |
|------|---------|-----------------|
| **Primary** | Main accent color | H1 headings, focused panel borders, command palette selection, status bar mode indicator, accent bars, dialog borders |
| **Secondary** | Secondary accent | H2 headings, wikilinks, link text, file icons, tag backgrounds |
| **Accent** | Warm accent / highlight | Active line number, list markers, peach highlights, unfocused selection |
| **Warning** | Caution / attention | Warning callouts, yellow markers, checkbox (todo) |
| **Success** | Positive / complete | Completed checkboxes, inline code, success messages, green indicators |
| **Error** | Negative / danger | Error messages, deletions, red markers, broken links |
| **Info** | Informational / cool accent | H3 headings, info callouts, blue/cyan hints, search highlights |

### Text Hierarchy (3)

| Role | Purpose | Where It Appears |
|------|---------|-----------------|
| **Text** | Primary text color | Body text, editor content, dialog text |
| **Subtext** | Secondary text | Descriptions, italic text, secondary labels |
| **Dim** | Tertiary / muted text | Comments, disabled items, hints, overlay backgrounds, help text |

### Surface Hierarchy (6)

| Role | Purpose | Where It Appears |
|------|---------|-----------------|
| **Surface2** | Lightest surface | Line number column background |
| **Surface1** | Mid surface | Unfocused panel borders, table borders |
| **Surface0** | Darkest surface | Code block backgrounds, input field backgrounds, search bar |
| **Base** | Main background | Editor background, sidebar background, panel backgrounds |
| **Mantle** | Status area | Status bar background |
| **Crust** | Footer area | Help bar background, tooltip backgrounds |

### Design Principles

- **Dark themes:** `Base` is dark, `Text` is light. Surface values are between Base and Text.
- **Light themes:** `Base` is light, `Text` is dark. Surface values are between Base and Text.
- **Contrast:** Ensure sufficient contrast between Text and Base (WCAG AA minimum: 4.5:1).
- **Accent harmony:** Primary, Secondary, and Accent should be visually distinct but harmonious.

---

## Style Properties

Beyond colors, themes define 5 style properties that affect the overall look and feel:

### Border

Controls the border style used for panels and dialogs.

| Value | Appearance | Description |
|-------|------------|-------------|
| `"rounded"` | `╭─╮` `│ │` `╰─╯` | Rounded corners (default, most themes) |
| `"normal"` | `┌─┐` `│ │` `└─┘` | Standard square corners |
| `"double"` | `╔═╗` `║ ║` `╚═╝` | Double-line borders |
| `"thick"` | `┏━┓` `┃ ┃` `┗━┛` | Thick/heavy borders |
| `"hidden"` | (none) | No visible borders |

### Density

Controls padding within panels.

| Value | Description | Best For |
|-------|-------------|----------|
| `"compact"` | Minimal padding | Small terminals, dense information display |
| `"normal"` | Standard padding | Most use cases |
| `"spacious"` | Extra padding | Large screens, readability |

### AccentBar

The character used for the sidebar selection indicator and other accent markers.

| Value | Appearance | Used In |
|-------|------------|---------|
| `"┃"` | Thick vertical bar | Catppuccin, Rose Pine, Monokai Pro |
| `"▎"` | Thin left bar | Tokyo Night, Kanagawa, Poimandres |
| `"▌"` | Half-block bar | Dracula, Synthwave, Cobalt2, Oxocarbon |
| `"█"` | Full block | Gruvbox |
| `"│"` | Thin vertical line | Nord, Solarized, Vesper, Zenburn, Amber |
| `">"` | Right arrow | Min-light |

### Separator

The character used for horizontal separators and dividers.

| Value | Appearance | Used In |
|-------|------------|---------|
| `"─"` | Standard horizontal line | Most themes |
| `"━"` | Thick horizontal line | Gruvbox, Kanagawa, Cobalt2, Oxocarbon |
| `"═"` | Double horizontal line | Synthwave-84 |
| `"╌"` | Dashed line | Rose Pine, Poimandres |
| `"·"` | Middle dot | Min-light |

### LinkUnderline

Boolean controlling whether wikilinks and other links are rendered with underlines.

| Value | Description | Used In |
|-------|-------------|---------|
| `true` | Links are underlined | Most themes |
| `false` | Links are not underlined | Gruvbox, Solarized, Vesper, Vitesse, Min-light, Matrix, Zenburn, Amber |

---

## Applying Themes

### From Settings

1. Press `Ctrl+,` to open Settings
2. Navigate to "Theme"
3. Use `Left`/`Right` arrows or `Enter` to cycle through themes
4. The theme applies immediately as you cycle

### From Command Palette

1. Press `Ctrl+X` to open Command palette
2. Type "Theme Editor"
3. Create or modify a theme
4. Save and apply

### From Config File

Edit `~/.config/granit/config.json`:

```json
{
  "theme": "tokyo-night"
}
```

Or per-vault in `<vault>/.granit.json`:

```json
{
  "theme": "github-light"
}
```

### Programmatic (for developers)

In Go code, themes are applied via:

```go
ApplyTheme("tokyo-night")
```

This updates all package-level color variables and rebuilds every Lip Gloss style, making the change visible immediately across all components.

---

## Theme Gallery

### Recommended Pairings

| Use Case | Dark Theme | Light Theme |
|----------|------------|-------------|
| General purpose | `catppuccin-mocha` | `catppuccin-latte` |
| Development | `tokyo-night` | `github-light` |
| Writing | `rose-pine` | `rose-pine-dawn` |
| Minimal | `vitesse-dark` | `min-light` |
| Retro | `gruvbox-dark` | `solarized-light` |
| Warm tones | `vesper` | `ayu-light` |
| Vibrant | `dracula` | `github-light` |
| Monochrome | `matrix` or `amber` | `min-light` |
| Low-contrast | `zenburn` | `solarized-light` |
