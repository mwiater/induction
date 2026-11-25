// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"context"
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pterm/pterm"
)

// consoleUITheme is the single source of truth for console colors and text
// styles. Keep visual changes here so they can be compared without hunting
// through the rendering code.
// consoleUITheme is an internal data type used by this package.
type consoleUITheme struct {
	// mainHeader stores the value represented by this member.
	mainHeader lipgloss.Style
	// mainUserPrompt stores the value represented by this member.
	mainUserPrompt lipgloss.Style
	// mainAssistantReasoning stores the value represented by this member.
	mainAssistantReasoning lipgloss.Style
	// mainAssistantContent stores the value represented by this member.
	mainAssistantContent lipgloss.Style
	// sidebarHeader stores the value represented by this member.
	sidebarHeader lipgloss.Style
	// sidebarBackground stores the value represented by this member.
	sidebarBackground lipgloss.Style
	// sidebarText stores the value represented by this member.
	sidebarText lipgloss.Style
	// footerBarMCP stores the value represented by this member.
	footerBarMCP lipgloss.Style
	// footerBarLiveMetrics stores the value represented by this member.
	footerBarLiveMetrics lipgloss.Style
	// footerBarPipeline stores the value represented by this member.
	footerBarPipeline lipgloss.Style
	// footerBarKeyBindings stores the value represented by this member.
	footerBarKeyBindings lipgloss.Style
	// mainError stores the value represented by this member.
	mainError lipgloss.Style
	// inputCursor stores the value represented by this member.
	inputCursor lipgloss.Style
	// mcpToolGroup stores the value represented by this member.
	mcpToolGroup lipgloss.Style
	// mcpToolName stores the value represented by this member.
	mcpToolName lipgloss.Style
	// mcpToolDescription stores the value represented by this member.
	mcpToolDescription lipgloss.Style
	// mcpActivity stores the value represented by this member.
	mcpActivity lipgloss.Style
}

// renderWhiteIcon keeps UI icons visually consistent while allowing the
// surrounding label and content to retain their existing text styles.
func renderWhiteIcon(icon string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Render(icon)
}

// renderIconStyled performs the internal operation described by its signature.
func renderIconStyled(icon, color string, style lipgloss.Style) string {
	iconStyle := style
	return iconStyle.Foreground(lipgloss.Color(color)).Width(0).Render(icon)
}

// renderIconBar performs the internal operation described by its signature.
func renderIconBar(icon, color, content string, style lipgloss.Style) string {
	return renderIconStyled(" "+icon, color, style) + style.Render(" "+strings.TrimLeft(content, " "))
}

// renderFullWidthHeader performs the internal operation described by its signature.
func renderFullWidthHeader(icon, title string, style lipgloss.Style, width int) string {
	base := style.Padding(0)
	iconStyle := base.Foreground(lipgloss.Color("#FFFFFF"))
	iconText := iconStyle.Render(" " + icon)
	titleText := base.Render(" " + title)
	remaining := max(0, width-lipgloss.Width(iconText)-lipgloss.Width(titleText))
	return iconText + titleText + base.Render(strings.Repeat(" ", remaining))
}

// defaultConsoleUITheme holds package-level state used internally by this package.
var defaultConsoleUITheme = consoleUITheme{
	// Keep the core palette to the standard ANSI colors. Kitty and other
	// terminals can have different advertised color profiles; these values
	// preserve the intended contrast instead of collapsing into blue-on-blue.
	mainHeader:             lipgloss.NewStyle().Foreground(lipgloss.Color("#7DCFFF")).Background(lipgloss.Color("#3B4261")).Bold(true).Padding(0, 1),
	mainUserPrompt:         lipgloss.NewStyle().Foreground(lipgloss.Color("#89B4FA")),
	mainAssistantReasoning: lipgloss.NewStyle().Foreground(lipgloss.Color("#A9B1D6")),
	mainAssistantContent:   lipgloss.NewStyle().Foreground(lipgloss.Color("#94E2D5")),
	sidebarHeader:          lipgloss.NewStyle().Foreground(lipgloss.Color("#333333")).Background(lipgloss.Color("#A9B1D6")).Bold(true).Padding(0, 1),
	sidebarBackground:      lipgloss.NewStyle().Background(lipgloss.Color("#333333")),
	sidebarText:            lipgloss.NewStyle().Foreground(lipgloss.Color("#A9B1D6")).Background(lipgloss.Color("#333333")).Padding(1, 2),
	footerBarMCP:           lipgloss.NewStyle().Foreground(lipgloss.Color("#1F2335")).Background(lipgloss.Color("#FF9E64")).Bold(true),
	footerBarLiveMetrics:   lipgloss.NewStyle().Foreground(lipgloss.Color("#15161E")).Background(lipgloss.Color("#7AA2F7")).Bold(true),
	footerBarPipeline:      lipgloss.NewStyle().Foreground(lipgloss.Color("#1A2418")).Background(lipgloss.Color("#9ECE6A")).Bold(true),
	footerBarKeyBindings:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#1F2335")),
	mainError:              lipgloss.NewStyle().Foreground(lipgloss.Color("#F7768E")),
	inputCursor:            lipgloss.NewStyle().Reverse(true),
	mcpToolGroup:           lipgloss.NewStyle().Foreground(lipgloss.Color("#7DCFFF")).Bold(true),
	mcpToolName:            lipgloss.NewStyle().Foreground(lipgloss.Color("#98C379")),
	mcpToolDescription:     lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")),
	mcpActivity:            lipgloss.NewStyle().Foreground(lipgloss.Color("#FF9E64")),
}

// consoleFooterKeyBindings identifies a fixed value used internally by this package.
const consoleFooterKeyBindings = "   Ctrl+S Save | Ctrl+L Load Chat | Ctrl+M Load Model | Ctrl+1 Model Info | Ctrl+C Quit"

// renderFooterBar applies a themed sticky footer bar and fills the available
// terminal row without using the final column, which can cause wrapping.
func (t consoleUITheme) renderFooterBar(icon, content string, mcp bool) string {
	if width := pterm.GetTerminalWidth() - 1; width > len(content) {
		content = fmt.Sprintf("%-*s", width, content)
	}
	style := t.footerBarLiveMetrics
	if mcp {
		style = t.footerBarMCP
	}
	return renderIconBar(icon, "#FFFF00", content, style)
}

// RunConsoleThemePreview displays one sample of every console theme element
// and exits after rendering it once.
func RunConsoleThemePreview(ctx context.Context, in io.Reader, out io.Writer) error {
	_, err := tea.NewProgram(consoleThemePreview{}, tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out)).Run()
	return err
}

// consoleThemePreview is an internal data type used by this package.
type consoleThemePreview struct{}

// Init is an exported operation provided by this package.
func (consoleThemePreview) Init() tea.Cmd { return tea.Quit }

// Update is an exported operation provided by this package.
func (consoleThemePreview) Update(tea.Msg) (tea.Model, tea.Cmd) {
	return consoleThemePreview{}, nil
}

// View is an exported operation provided by this package.
func (consoleThemePreview) View() string {
	return lipgloss.JoinVertical(
		lipgloss.Left,
		defaultConsoleUITheme.mainHeader.Render("mainHeader: MAIN CONTENT HEADER"),
		defaultConsoleUITheme.mainUserPrompt.Render("mainUserPrompt: USER: sample user message"),
		defaultConsoleUITheme.mainAssistantReasoning.Render("mainAssistantReasoning: ASSISTANT: sample thinking output"),
		defaultConsoleUITheme.mainAssistantContent.Render("mainAssistantContent: ASSISTANT: sample final output"),
		defaultConsoleUITheme.sidebarHeader.Render("sidebarHeader: SIDEBAR HEADER"),
		defaultConsoleUITheme.sidebarBackground.Render(defaultConsoleUITheme.sidebarText.Render("sidebarBackground + sidebarText: sample sidebar information")),
		defaultConsoleUITheme.footerBarMCP.Render("footerBarMCP: sample MCP status"),
		defaultConsoleUITheme.footerBarLiveMetrics.Render("footerBarLiveMetrics: [Induction: Live Metrics] sample status"),
		defaultConsoleUITheme.footerBarPipeline.Render("footerBarPipeline: pipeline.yaml | Pipeline step 1/2: extract"),
		defaultConsoleUITheme.footerBarKeyBindings.Render("footerBarKeyBindings: [^C] Quit   [^R] Refresh   [Tab] Next"),
		defaultConsoleUITheme.mainError.Render("mainError: sample error message"),
		defaultConsoleUITheme.inputCursor.Render("inputCursor: sample input cursor"),
	)
}

// IconSet defines a collection of monochrome symbols for terminal dashboards.
// IconSet is an exported type in this package's public data model.
type IconSet struct {
	// Navigation & Pointers
	PointerRight, PointerLeft, PointerUp, PointerDown string
	// ArrowLeft stores the value represented by this member.
	// ArrowRight stores the value represented by this member.
	// ArrowDown stores the value represented by this member.
	// ArrowUp stores the value represented by this member.
	ArrowUp, ArrowDown, ArrowRight, ArrowLeft string
	// ChevronLeft stores the value represented by this member.
	// ChevronRight stores the value represented by this member.
	// ChevronDown stores the value represented by this member.
	// ChevronUp stores the value represented by this member.
	ChevronUp, ChevronDown, ChevronRight, ChevronLeft string
	// TriangleLeft stores the value represented by this member.
	// TriangleRight stores the value represented by this member.
	// TriangleDown stores the value represented by this member.
	// TriangleUp stores the value represented by this member.
	TriangleUp, TriangleDown, TriangleRight, TriangleLeft string
	// End stores the value represented by this member.
	// Home stores the value represented by this member.
	// DoubleArrowLeft stores the value represented by this member.
	// DoubleArrowRight stores the value represented by this member.
	DoubleArrowRight, DoubleArrowLeft, Home, End string

	// Status & Validation
	Check, Cross, Warning, Info, Question string
	// Error stores the value represented by this member.
	// Pending stores the value represented by this member.
	// Failure stores the value represented by this member.
	// Success stores the value represented by this member.
	Success, Failure, Pending, Error string
	// Minus stores the value represented by this member.
	// Plus stores the value represented by this member.
	// Lightning stores the value represented by this member.
	// Sparkle stores the value represented by this member.
	Sparkle, Lightning, Plus, Minus string
	// Ellipsis stores the value represented by this member.
	// Dot stores the value represented by this member.
	Dot, Ellipsis string
	// Neutral stores the value represented by this member.
	// Blocked stores the value represented by this member.
	// Muted stores the value represented by this member.
	// Complete stores the value represented by this member.
	// Started stores the value represented by this member.
	Started, Complete, Muted, Blocked, Neutral string

	// Structure & Borders
	SeparatorVertical, SeparatorHorizontal string
	// SeparatorDoubleHorizontal stores the value represented by this member.
	// SeparatorDoubleVertical stores the value represented by this member.
	SeparatorDoubleVertical, SeparatorDoubleHorizontal string
	// CornerBottomRight stores the value represented by this member.
	// CornerBottomLeft stores the value represented by this member.
	// CornerTopRight stores the value represented by this member.
	// CornerTopLeft stores the value represented by this member.
	CornerTopLeft, CornerTopRight, CornerBottomLeft, CornerBottomRight string
	// CornerTopRightRounded stores the value represented by this member.
	// CornerTopLeftRounded stores the value represented by this member.
	CornerTopLeftRounded, CornerTopRightRounded string
	// CornerBottomRightRounded stores the value represented by this member.
	// CornerBottomLeftRounded stores the value represented by this member.
	CornerBottomLeftRounded, CornerBottomRightRounded string
	// JunctionBottom stores the value represented by this member.
	// JunctionTop stores the value represented by this member.
	// JunctionRight stores the value represented by this member.
	// JunctionLeft stores the value represented by this member.
	JunctionLeft, JunctionRight, JunctionTop, JunctionBottom string
	// TRight stores the value represented by this member.
	// TLeft stores the value represented by this member.
	// JunctionCross stores the value represented by this member.
	JunctionCross, TLeft, TRight string
	// TTop stores the value represented by this member.
	TTop string

	// Toggles & Selection
	Bullet, BulletHollow, RadioOn, RadioOff, CheckboxOn, CheckboxOff string
	// StarHollow stores the value represented by this member.
	// Star stores the value represented by this member.
	// DiamondHollow stores the value represented by this member.
	// Diamond stores the value represented by this member.
	// SquareHollow stores the value represented by this member.
	// Square stores the value represented by this member.
	Square, SquareHollow, Diamond, DiamondHollow, Star, StarHollow string
	// Bookmark stores the value represented by this member.
	// Pin stores the value represented by this member.
	// Unselected stores the value represented by this member.
	// Selected stores the value represented by this member.
	Selected, Unselected, Pin, Bookmark string
	// Grip stores the value represented by this member.
	// Handle stores the value represented by this member.
	// FlagHollow stores the value represented by this member.
	// Flag stores the value represented by this member.
	Flag, FlagHollow, Handle, Grip string

	// Progress & Charts
	BlockFull, BlockDark, BlockMedium, BlockLight string
	// BlockLower stores the value represented by this member.
	// BlockUpper stores the value represented by this member.
	// BlockRight stores the value represented by this member.
	// BlockLeft stores the value represented by this member.
	BlockLeft, BlockRight, BlockUpper, BlockLower string
	// BarEmpty stores the value represented by this member.
	// BarVertical stores the value represented by this member.
	// BarHorizontal stores the value represented by this member.
	BarHorizontal, BarVertical, BarEmpty string
	// Clock stores the value represented by this member.
	// Hourglass stores the value represented by this member.
	// Reload stores the value represented by this member.
	// Refresh stores the value represented by this member.
	Refresh, Reload, Hourglass, Clock string
	// ChartLine stores the value represented by this member.
	// ChartBar stores the value represented by this member.
	// TrendDown stores the value represented by this member.
	// TrendUp stores the value represented by this member.
	TrendUp, TrendDown, ChartBar, ChartLine string
	// ChartArea stores the value represented by this member.
	ChartArea string
}

// DefaultUnicode provides the standard rich UI experience.
var DefaultUnicode = IconSet{
	// Navigation & Pointers
	PointerRight:     "❯",
	PointerLeft:      "❮",
	PointerUp:        "⌃",
	PointerDown:      "⌄",
	ArrowUp:          "↑",
	ArrowDown:        "↓",
	ArrowRight:       "→",
	ArrowLeft:        "←",
	ChevronUp:        "˄",
	ChevronDown:      "˅",
	ChevronRight:     "›",
	ChevronLeft:      "‹",
	TriangleUp:       "▲",
	TriangleDown:     "▼",
	TriangleRight:    "▶",
	TriangleLeft:     "◀",
	DoubleArrowRight: "»",
	DoubleArrowLeft:  "«",
	Home:             "⌂",
	End:              "⌁",

	// Status & Validation
	Check:     "✔",
	Cross:     "✘",
	Warning:   "!",
	Info:      "ℹ",
	Question:  "?",
	Success:   "✓",
	Failure:   "✗",
	Pending:   "…",
	Error:     "⊗",
	Sparkle:   "✦",
	Lightning: "ϟ",
	Plus:      "+",
	Minus:     "−",
	Dot:       "·",
	Ellipsis:  "...",
	Started:   "◌",
	Complete:  "☑",
	Muted:     "⊖",
	Blocked:   "⊘",
	Neutral:   "•",

	// Structure & Borders (Box Drawing)
	SeparatorVertical:         "│",
	SeparatorHorizontal:       "─",
	SeparatorDoubleVertical:   "║",
	SeparatorDoubleHorizontal: "═",
	CornerTopLeft:             "┌",
	CornerTopRight:            "┐",
	CornerBottomLeft:          "└",
	CornerBottomRight:         "┘",
	CornerTopLeftRounded:      "╭",
	CornerTopRightRounded:     "╮",
	CornerBottomLeftRounded:   "╰",
	CornerBottomRightRounded:  "╯",
	JunctionLeft:              "┤",
	JunctionRight:             "├",
	JunctionTop:               "┴",
	JunctionBottom:            "┬",
	JunctionCross:             "┼",
	TLeft:                     "┬",
	TRight:                    "┴",
	TTop:                      "┤",

	// Toggles & Selection
	Bullet:        "●",
	BulletHollow:  "○",
	RadioOn:       "◉",
	RadioOff:      "◯",
	CheckboxOn:    "☒",
	CheckboxOff:   "☐",
	Square:        "■",
	SquareHollow:  "□",
	Diamond:       "◆",
	DiamondHollow: "◇",
	Star:          "★",
	StarHollow:    "☆",
	Selected:      "☑",
	Unselected:    "☐",
	Pin:           "⌖",
	Bookmark:      "▮",
	Flag:          "⚑",
	FlagHollow:    "⚐",
	Handle:        "⋮⋮",
	Grip:          "⠿",

	// Progress & Charts
	BlockFull:     "█",
	BlockDark:     "▓",
	BlockMedium:   "▒",
	BlockLight:    "░",
	BlockLeft:     "▌",
	BlockRight:    "▐",
	BlockUpper:    "▀",
	BlockLower:    "▄",
	BarHorizontal: "━",
	BarVertical:   "┃",
	BarEmpty:      "╌",
	Refresh:       "↻",
	Reload:        "⟳",
	Hourglass:     "⧖",
	Clock:         "◷",
	TrendUp:       "↗",
	TrendDown:     "↘",
	ChartBar:      "▂▅▇",
	ChartLine:     "╱╲╱",
	ChartArea:     "▁▃▆",
}
