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
type consoleUITheme struct {
	mainHeader             lipgloss.Style
	mainUserPrompt         lipgloss.Style
	mainAssistantReasoning lipgloss.Style
	mainAssistantContent   lipgloss.Style
	sidebarHeader          lipgloss.Style
	sidebarBackground      lipgloss.Style
	sidebarText            lipgloss.Style
	footerBarMCP           lipgloss.Style
	footerBarLiveMetrics   lipgloss.Style
	footerBarKeyBindings   lipgloss.Style
	mainError              lipgloss.Style
	inputCursor            lipgloss.Style
	mcpToolGroup           lipgloss.Style
	mcpToolName            lipgloss.Style
	mcpToolDescription     lipgloss.Style
	mcpActivity            lipgloss.Style
}

// renderWhiteIcon keeps UI icons visually consistent while allowing the
// surrounding label and content to retain their existing text styles.
func renderWhiteIcon(icon string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Render(icon)
}

// renderWhiteIconStyled places a white icon on the same styled surface as its
// neighboring text, without changing that text's color.
func renderWhiteIconStyled(icon string, style lipgloss.Style) string {
	return renderIconStyled(icon, "#FFFFFF", style)
}

func renderIconStyled(icon, color string, style lipgloss.Style) string {
	iconStyle := style
	return iconStyle.Foreground(lipgloss.Color(color)).Width(0).Render(icon)
}

func renderWhiteIconBar(icon, content string, style lipgloss.Style) string {
	return renderIconBar(icon, "#FFFFFF", content, style)
}

func renderIconBar(icon, color, content string, style lipgloss.Style) string {
	return renderIconStyled(" "+icon, color, style) + style.Render(" "+strings.TrimLeft(content, " "))
}

func renderFullWidthHeader(icon, title string, style lipgloss.Style, width int) string {
	base := style.Padding(0)
	iconStyle := base.Foreground(lipgloss.Color("#FFFFFF"))
	iconText := iconStyle.Render(" " + icon)
	titleText := base.Render(" " + title)
	remaining := max(0, width-lipgloss.Width(iconText)-lipgloss.Width(titleText))
	return iconText + titleText + base.Render(strings.Repeat(" ", remaining))
}

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
	footerBarMCP:           lipgloss.NewStyle().Foreground(lipgloss.Color("#FF9E64")).Background(lipgloss.Color("#1F2335")).Bold(true),
	footerBarLiveMetrics:   lipgloss.NewStyle().Foreground(lipgloss.Color("#15161E")).Background(lipgloss.Color("#7AA2F7")).Bold(true),
	footerBarKeyBindings:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#1F2335")),
	mainError:              lipgloss.NewStyle().Foreground(lipgloss.Color("#F7768E")),
	inputCursor:            lipgloss.NewStyle().Reverse(true),
	mcpToolGroup:           lipgloss.NewStyle().Foreground(lipgloss.Color("#7DCFFF")).Bold(true),
	mcpToolName:            lipgloss.NewStyle().Foreground(lipgloss.Color("#98C379")),
	mcpToolDescription:     lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")),
	mcpActivity:            lipgloss.NewStyle().Foreground(lipgloss.Color("#FF9E64")),
}

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
// and exits when the user presses a key.
func RunConsoleThemePreview(ctx context.Context, in io.Reader, out io.Writer) error {
	_, err := tea.NewProgram(consoleThemePreview{}, tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out)).Run()
	return err
}

type consoleThemePreview struct{}

func (consoleThemePreview) Init() tea.Cmd { return nil }

func (consoleThemePreview) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		return consoleThemePreview{}, tea.Quit
	}
	return consoleThemePreview{}, nil
}

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
		defaultConsoleUITheme.footerBarKeyBindings.Render("footerBarKeyBindings: [^C] Quit   [^R] Refresh   [Tab] Next"),
		defaultConsoleUITheme.mainError.Render("mainError: sample error message"),
		defaultConsoleUITheme.inputCursor.Render("inputCursor: sample input cursor"),
		"\nPress any key to exit.",
	)
}

// IconSet defines a collection of monochrome symbols for terminal dashboards.
type IconSet struct {
	// Navigation & Pointers
	PointerRight, PointerLeft, PointerUp, PointerDown     string
	ArrowUp, ArrowDown, ArrowRight, ArrowLeft             string
	ChevronUp, ChevronDown, ChevronRight, ChevronLeft     string
	TriangleUp, TriangleDown, TriangleRight, TriangleLeft string
	DoubleArrowRight, DoubleArrowLeft, Home, End          string

	// Status & Validation
	Check, Cross, Warning, Info, Question      string
	Success, Failure, Pending, Error           string
	Sparkle, Lightning, Plus, Minus            string
	Dot, Ellipsis                              string
	Started, Complete, Muted, Blocked, Neutral string

	// Structure & Borders
	SeparatorVertical, SeparatorHorizontal                             string
	SeparatorDoubleVertical, SeparatorDoubleHorizontal                 string
	CornerTopLeft, CornerTopRight, CornerBottomLeft, CornerBottomRight string
	CornerTopLeftRounded, CornerTopRightRounded                        string
	CornerBottomLeftRounded, CornerBottomRightRounded                  string
	JunctionLeft, JunctionRight, JunctionTop, JunctionBottom           string
	JunctionCross, TLeft, TRight                                       string
	TTop                                                               string

	// Toggles & Selection
	Bullet, BulletHollow, RadioOn, RadioOff, CheckboxOn, CheckboxOff string
	Square, SquareHollow, Diamond, DiamondHollow, Star, StarHollow   string
	Selected, Unselected, Pin, Bookmark                              string
	Flag, FlagHollow, Handle, Grip                                   string

	// Progress & Charts
	BlockFull, BlockDark, BlockMedium, BlockLight string
	BlockLeft, BlockRight, BlockUpper, BlockLower string
	BarHorizontal, BarVertical, BarEmpty          string
	Refresh, Reload, Hourglass, Clock             string
	TrendUp, TrendDown, ChartBar, ChartLine       string
	ChartArea                                     string
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
