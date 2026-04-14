/*
 * ████████▒ ████▒   ████▒ ████████▒     ██████▒   ████████▒
 * ████████████████▒ ████▒ ██████████▒ ██████████▒ ██████████▒
 * ████▒ ████▒ ████▒ ████▒ ████▒ ████▒ ████▒ ████▒ ████▒ ████▒
 * ████▒ ████▒ ████▒ ████▒ ████▒ ████▒ ██████████▒ ██████████▒
 * ████▒ ████▒ ████▒ ████▒ ████▒ ████▒ ██████████▒ ████████▒
 * ████▒ ████▒ ████▒ ████▒ ████▒ ████▒ ████▒       ████▒ ████▒
 * ████▒ ████▒ ████▒ ████▒ ████▒ ████▒ ██████████▒ ████▒ ████▒
 * ████▒ ████▒ ████▒ ████▒ ████▒ ████▒   ████████▒ ████▒ ████▒
 *
 * A tiny TUI toy about a miner digging through an endless ASCII world of tiles
 * Each tile has a type with different mining duration and point value
 * Each tile is represented by a colored character
 * Each row of tiles has a chance to contain a special tile
 * Miner scans current row for special tile and mines its way to it, then digs down to the next row
 */

package main

import (
	"fmt"
	"image/color"
	"math/rand"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var (
	blendWidth  = 69
	blendHeight = getLogoHeight()

	logoBlend     = lipgloss.Blend2D(blendWidth, blendHeight, 90.0, color.Black, lipgloss.Color("1"))
	viewStyle     = lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(lipgloss.Color("6"))
	topLabelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))

	tileStyle = lipgloss.NewStyle()

	minerColor = lipgloss.Color("1")

	boldLabel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
)

var (
	cameraFollowOffset = 10 // will be updated based on terminal height on window resize

	tickInterval = 100 * time.Millisecond
	tickMillis   = int64(tickInterval / time.Millisecond)

	specialTileChance = 50 // %
)

const (
	titleLogo = "" +
		"████████▒ ████▒   ████▒ ████████▒     ██████▒   ████████▒  \n" +
		"████████████████▒ ████▒ ██████████▒ ██████████▒ ██████████▒\n" +
		"████▒ ████▒ ████▒ ████▒ ████▒ ████▒ ████▒ ████▒ ████▒ ████▒\n" +
		"████▒ ████▒ ████▒ ████▒ ████▒ ████▒ ██████████▒ ██████████▒\n" +
		"████▒ ████▒ ████▒ ████▒ ████▒ ████▒ ██████████▒ ████████▒  \n" +
		"████▒ ████▒ ████▒ ████▒ ████▒ ████▒ ████▒       ████▒ ████▒\n" +
		"████▒ ████▒ ████▒ ████▒ ████▒ ████▒ ██████████▒ ████▒ ████▒\n" +
		"████▒ ████▒ ████▒ ████▒ ████▒ ████▒   ████████▒ ████▒ ████▒\n"

	intervalStep          = 10 * time.Millisecond
	specialTileChanceStep = 5

	// Layout
	uiTopSpacing      = 4
	defaultWorldWidth = 20

	// Miner
	noTarget = -1

	// Tile render thresholds
	damageHigh   = 0.75
	damageMedium = 0.5
	damageLow    = 0.25

	// Characters
	charFull  = "███"
	charHeavy = "▓▓▓"
	charMid   = "▒▒▒"
	charLight = "░░░"
	charMiner = " ▄ "
)

const (
	TileStone = iota
	TileCoal
	TileIron
	TileGold
	TileDiamond
)
const defaultSpecialTileType = TileCoal
const (
	initializing state = iota
	ready
)

type state int

// Colors from https://en.wikipedia.org/wiki/ANSI_escape_code#8-bit
var tileTypes = []struct {
	name     string
	color    string
	value    int
	mineTime time.Duration
	weight   int
}{
	{"Stone", "8", 1, 200 * time.Millisecond, 0},
	{"Coal", "0", 6, 500 * time.Millisecond, 60},
	{"Iron", "5", 14, 1 * time.Second, 35},
	{"Gold", "3", 30, 2*time.Second + 500*time.Millisecond, 15},
	{"Diamond", "4", 50, 4 * time.Second, 5},
}

type tickMsg time.Time

type miner struct {
	x int
	y int

	score   int
	targetX int
}

type tile struct {
	tileType int
	duration int64 // remaining mining time in ms, 0 if mined
	mined    bool
}

type world struct {
	view viewport.Model

	width int

	tiles [][]tile
}

type camera struct {
	y int
}

type model struct {
	state      state
	started_at time.Time

	width  int
	height int

	miner  miner
	world  world
	camera camera

	legendView   viewport.Model
	settingsView viewport.Model
}

/*
 * Generate row of tiles, check against specialTileChance if row containts a special tile
 * If special tile is generated, pick random tile from the row and tile type based on weighted chance
 */
func (world *world) generateRow() {
	row := make([]tile, world.width)
	hasSpecial := rand.Intn(100) < specialTileChance
	specialIndex := -1
	if hasSpecial {
		specialIndex = rand.Intn(world.width)
	}
	specialTileType := defaultSpecialTileType
	if hasSpecial {
		totalWeight := 0
		for _, t := range tileTypes {
			if t.weight > 0 {
				totalWeight += t.weight
			}
		}
		r := rand.Intn(totalWeight)
		for i, t := range tileTypes {
			if t.weight > 0 {
				r -= t.weight
				if r < 0 {
					specialTileType = i
					break
				}
			}
		}
	}

	for i := 0; i < world.width; i++ {
		if i == specialIndex {
			row[i] = tile{tileType: specialTileType, duration: tileTypes[specialTileType].mineTime.Milliseconds()}
		} else {
			row[i] = tile{tileType: TileStone, duration: tileTypes[TileStone].mineTime.Milliseconds()} // stone
		}
	}
	world.tiles = append(world.tiles, row)
}

func (cam *camera) follow(miner miner) {
	if miner.y-cam.y > cameraFollowOffset {
		cam.y = miner.y - cameraFollowOffset
	}
}

func (m *miner) digDown(world *world) {
	var tile = &world.tiles[m.y][m.x]
	if !tile.mined {
		tile.duration -= tickMillis
	}

	if !tile.mined && tile.duration <= 0 {
		m.score += tileTypes[tile.tileType].value
		tile.mined = true
		m.y++
		m.targetX = noTarget
	}
}

func (m *miner) update(world *world) {
	if m.targetX == noTarget && m.y > 0 {
		for x, tile := range world.tiles[m.y-1] {
			if tile.tileType > TileStone && !tile.mined {
				m.targetX = x
				break
			}
		}
	}

	if m.targetX == noTarget {
		m.digDown(world)
		return
	}

	if m.y == 0 {
		m.digDown(world)
	} else {
		if m.x < m.targetX {
			var tile = &world.tiles[m.y-1][m.x+1]
			if !tile.mined {
				tile.duration -= tickMillis
			}

			if !tile.mined && tile.duration <= 0 {
				m.score += tileTypes[tile.tileType].value
				tile.mined = true
				m.x++
			}
		} else if m.x > m.targetX {
			var tile = &world.tiles[m.y-1][m.x-1]
			if !tile.mined {
				tile.duration -= tickMillis
			}

			if !tile.mined && tile.duration <= 0 {
				m.score += tileTypes[tile.tileType].value
				tile.mined = true
				m.x--
			}
		} else {
			m.digDown(world)
		}
	}
}

func (t tile) Render() string {
	percentage := float64(t.duration) / float64(tileTypes[t.tileType].mineTime.Milliseconds())

	switch {
	case percentage > damageHigh:
		return charFull
	case percentage > damageMedium:
		return charHeavy
	case percentage > damageLow:
		return charMid
	default:
		return charLight
	}
}

func (m *model) renderWorld() string {
	left := topLabelStyle.Render(fmt.Sprintf("Score: %d", m.miner.score))
	right := topLabelStyle.Render(fmt.Sprintf("Floor: %d", m.miner.y))

	var t string
	elapsed := time.Since(m.started_at)
	if elapsed < time.Minute {
		t = fmt.Sprintf("%d", int(elapsed.Seconds()))
	} else if elapsed < time.Hour {
		t = fmt.Sprintf("%02d:%02d", int(elapsed.Minutes()), int(elapsed.Seconds())%60)
	} else {
		t = fmt.Sprintf("%02d:%02d:%02d", int(elapsed.Hours()), int(elapsed.Minutes())%60, int(elapsed.Seconds())%60)
	}

	middle := lipgloss.PlaceHorizontal((m.world.width*lipgloss.Width(charFull))-lipgloss.Width(left)-lipgloss.Width(right),
		lipgloss.Center,
		topLabelStyle.Render(t))

	top := lipgloss.PlaceHorizontal(
		m.world.width*lipgloss.Width(charFull),
		lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Top, left, middle, right),
	)

	var s strings.Builder

	s.WriteString(top)

	for range uiTopSpacing - 1 {
		s.WriteString("\n")
	}

	miner := tileStyle.Foreground(minerColor).Render(charMiner)

	if m.miner.y == 0 {
		s.WriteString(strings.Repeat(" ", m.miner.x*lipgloss.Width(charFull)))
		s.WriteString(miner)
	}
	s.WriteString("\n")

	for y := m.camera.y; y < min(len(m.world.tiles), m.camera.y+m.height-uiTopSpacing-1); y++ {
		for x := 0; x < m.world.width; x++ {
			if m.miner.y > 0 && m.miner.x == x && m.miner.y-1 == y {
				s.WriteString(miner)
				continue
			}

			var tile = &m.world.tiles[y][x]

			if !tile.mined {
				s.WriteString(tileStyle.Foreground(lipgloss.Color(tileTypes[tile.tileType].color)).Render(tile.Render()))
			} else {
				s.WriteString(strings.Repeat(" ", lipgloss.Width(charFull)))
			}
		}
		s.WriteString("\n")
	}

	m.world.view.SetContent(s.String())

	return m.world.view.View()
}

func (m *model) buildLegend() {
	var tiles strings.Builder

	for _, t := range tileTypes {
		fmt.Fprintf(&tiles, "%s %s (%d Point%s)\n", tileStyle.Foreground(lipgloss.Color(t.color)).Render(charFull), t.name, t.value, func() string {
			if t.value == 1 {
				return ""
			}
			return "s"
		}())
	}

	legend := lipgloss.NewStyle().Render(fmt.Sprintf("%s Miner\n%s", tileStyle.Foreground(minerColor).Render(charMiner), tiles.String()))

	m.legendView.SetContent(legend)
}

func (m *model) buildSettings() {
	var settings strings.Builder

	settings.WriteString(boldLabel.Italic(true).Render("Settings"))
	settings.WriteString("\n")

	settings.WriteString(fmt.Sprintf("%s %s\n", boldLabel.Render("Tick Interval"), tickInterval.String()))
	settings.WriteString(fmt.Sprintf("%s %d%%\n", boldLabel.Render("Special Tile Chance"), specialTileChance))

	// Keybinds
	settings.WriteString("\n")
	settings.WriteString(boldLabel.Italic(true).Render("Keybinds"))
	settings.WriteString("\n")
	settings.WriteString(fmt.Sprintf("%s Increase Tick Interval\n", boldLabel.Render("Up/K")))
	settings.WriteString(fmt.Sprintf("%s Decrease Tick Interval\n", boldLabel.Render("Down/J")))
	settings.WriteString(fmt.Sprintf("%s Increase Special Tile Chance\n", boldLabel.Render("Right/L")))
	settings.WriteString(fmt.Sprintf("%s Decrease Special Tile Chance\n", boldLabel.Render("Left/H")))
	settings.WriteString(fmt.Sprintf("%s Quit\n", boldLabel.Render("Q/Ctrl+C")))

	m.settingsView.SetContent(settings.String())
}

func renderLogo() string {
	lines := strings.Split(titleLogo, "\n")
	gradientContent := strings.Builder{}

	for y, line := range lines {
		if y >= blendHeight {
			break
		}
		x := 0
		for _, ch := range line {
			if x >= blendWidth {
				break
			}
			index := y*blendWidth + x
			if ch != ' ' {
				gradientContent.WriteString(
					lipgloss.NewStyle().
						Foreground(logoBlend[index]).
						Render(string(ch)),
				)
			} else {
				gradientContent.WriteString(" ")
			}
			x++
		}
		if y < len(lines)-2 {
			gradientContent.WriteString("\n")
		}
	}

	return gradientContent.String()
}

func getLogoHeight() int {
	return strings.Count(titleLogo, "\n")
}

func newModel() model {
	m := model{
		miner: miner{
			x:       0,
			y:       0,
			score:   0,
			targetX: noTarget,
		},
		world: world{
			view:  viewport.New(),
			tiles: make([][]tile, 0),
		},
		camera: camera{
			y: 0,
		},
		state:        initializing,
		legendView:   viewport.New(),
		settingsView: viewport.New(),
	}

	return m
}

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) Init() tea.Cmd {
	return tick(tickInterval)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		for len(m.world.tiles) <= m.camera.y+m.height {
			m.world.generateRow()
		}

		m.camera.follow(m.miner)

		m.miner.update(&m.world)

		return m, tick(tickInterval)

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			tickInterval += intervalStep
		case "down", "j":
			tickInterval = max(intervalStep, tickInterval-intervalStep)
		case "left", "h":
			if specialTileChance > 0 {
				specialTileChance -= specialTileChanceStep
			}
		case "right", "l":
			if specialTileChance < 100 {
				specialTileChance += specialTileChanceStep
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.state == initializing {
			m.started_at = time.Now()
			m.world.width = min(defaultWorldWidth, m.width)
			for i := 0; i < m.height; i++ {
				m.world.generateRow()
			}
			m.miner.x = m.world.width / 2
			m.state = ready
		}

		m.world.view.SetHeight(m.height - 2 - getLogoHeight())
		m.world.view.SetWidth(m.world.width * lipgloss.Width(charFull))
		m.legendView.SetHeight(1 + len(tileTypes))
		m.legendView.SetWidth(m.width - m.world.view.Width() - 4)
		m.settingsView.SetHeight(m.height - m.legendView.Height() - 4 - getLogoHeight())
		m.settingsView.SetWidth(m.legendView.Width())

		cameraFollowOffset = max(uiTopSpacing, m.world.view.Height()/2) - 2
	}
	return m, nil
}

func (m model) View() tea.View {
	var v tea.View
	v.AltScreen = true
	if m.state == initializing {
		v.SetContent(renderLogo())
	} else {
		m.buildLegend()
		m.buildSettings()

		s := lipgloss.JoinVertical(lipgloss.Top,
			lipgloss.PlaceHorizontal(m.width, lipgloss.Center, renderLogo()),
			lipgloss.JoinHorizontal(lipgloss.Top,
				viewStyle.Render(m.renderWorld()),
				lipgloss.JoinVertical(lipgloss.Top,
					viewStyle.Render(m.legendView.View()),
					viewStyle.Render(m.settingsView.View()),
				),
			))

		v.SetContent(s)
	}
	return v
}

func main() {
	p := tea.NewProgram(newModel())
	if _, err := p.Run(); err != nil {
		os.Exit(1)
	}
}
