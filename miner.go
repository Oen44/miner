/*
 * A tiny TUI toy about a miner digging through an endless ASCII world of tiles
 * Each tile has a type with different mining duration and point value
 * Each tile is represented by a colored character
 * Each row of tiles has a chance to contain a special tile
 * Miner scans current row for special tile and mines its way to it, then digs down to the next row
 */

package main

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	viewStyle = lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(lipgloss.Color("#681a10"))
)

const (
	tickInterval      = 100 * time.Millisecond
	tickMillis        = int64(tickInterval / time.Millisecond)
	specialTileChance = 50 // %

	// Camera
	cameraFollowOffset = 10

	// Layout
	uiTopSpacing      = 4
	defaultWorldWidth = 40

	// Miner
	noTargetX = -1

	// Tile render thresholds
	damageHigh   = 0.75
	damageMedium = 0.5
	damageLow    = 0.25

	// Characters
	charFull  = "█"
	charHeavy = "▓"
	charMid   = "▒"
	charLight = "░"
	charMiner = "▄"
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
	{"Stone", "240", 1, 200 * time.Millisecond, 0},
	{"Coal", "232", 6, 500 * time.Millisecond, 60},
	{"Iron", "130", 14, 1 * time.Second, 35},
	{"Gold", "220", 30, 2*time.Second + 500*time.Millisecond, 15},
	{"Diamond", "51", 50, 4 * time.Second, 5},
}

var tileStyle = lipgloss.NewStyle()

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

	legendView viewport.Model
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
		m.targetX = noTargetX
	}
}

func (m *miner) update(world *world) {
	if m.targetX == noTargetX {
		for x, tile := range world.tiles[m.y] {
			if tile.tileType > TileStone && !tile.mined {
				m.targetX = x
				break
			}
		}
	}

	if m.targetX == noTargetX {
		m.digDown(world)
		return
	}

	if m.y == 0 {
		if m.x < m.targetX {
			m.x++
		} else if m.x > m.targetX {
			m.x--
		} else {
			m.digDown(world)
		}
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
	left := fmt.Sprintf("Score: %d", m.miner.score)
	right := fmt.Sprintf("Floor: %d", m.miner.y)

	var t string
	elapsed := time.Since(m.started_at)
	if elapsed < time.Minute {
		t = fmt.Sprintf("%d", int(elapsed.Seconds()))
	} else if elapsed < time.Hour {
		t = fmt.Sprintf("%02d:%02d", int(elapsed.Minutes()), int(elapsed.Seconds())%60)
	} else {
		t = fmt.Sprintf("%02d:%02d:%02d", int(elapsed.Hours()), int(elapsed.Minutes())%60, int(elapsed.Seconds())%60)
	}

	middle := lipgloss.PlaceHorizontal(m.world.width-lipgloss.Width(left)-lipgloss.Width(right), lipgloss.Center, t)

	top := lipgloss.PlaceHorizontal(
		m.world.width,
		lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Top, left, middle, right),
	)

	var s strings.Builder

	s.WriteString(top)

	for range uiTopSpacing - 1 {
		s.WriteString("\n")
	}

	miner := tileStyle.Foreground(lipgloss.Color("15")).Render(charMiner)

	if m.miner.y == 0 {
		s.WriteString(strings.Repeat(" ", m.miner.x))
		s.WriteString(miner)
	}
	s.WriteString("\n")

	for y := m.camera.y; y < m.camera.y+m.height-uiTopSpacing-1; y++ {
		for x := 0; x < m.world.width; x++ {
			if m.miner.y > 0 && m.miner.x == x && m.miner.y-1 == y {
				s.WriteString(miner)
				continue
			}

			var tile = &m.world.tiles[y][x]

			if !tile.mined {
				s.WriteString(tileStyle.Foreground(lipgloss.Color(tileTypes[tile.tileType].color)).Render(tile.Render()))
			} else {
				s.WriteString(" ")
			}
		}
		s.WriteString("\n")
	}

	m.world.view.SetContent(s.String())

	return m.world.view.View()
}

func newModel() model {
	m := model{
		miner: miner{
			x:       0,
			y:       0,
			score:   0,
			targetX: noTargetX,
		},
		world: world{
			view:  viewport.New(0, 0),
			tiles: make([][]tile, 0),
		},
		camera: camera{
			y: 0,
		},
		state:      initializing,
		legendView: viewport.New(0, 0),
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

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC || msg.String() == "q" {
			return m, tea.Quit
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

		m.world.view.Height = m.height - 2
		m.world.view.Width = m.world.width
		m.legendView.Height = m.height - 2
		m.legendView.Width = m.width - m.world.view.Width - 4

	}
	return m, nil
}

func (m model) View() string {
	if m.state == initializing {
		return "Initializing..."
	}

	var tiles strings.Builder

	for _, t := range tileTypes {
		fmt.Fprintf(&tiles, "%s %s (%d Point%s)\n", tileStyle.Foreground(lipgloss.Color(t.color)).Render(charFull), t.name, t.value, func() string {
			if t.value == 1 {
				return ""
			}
			return "s"
		}())
	}

	legend := lipgloss.NewStyle().Render(fmt.Sprintf("%s Miner\n%s", tileStyle.Foreground(lipgloss.Color("15")).Render(charMiner), tiles.String()))

	m.legendView.SetContent(legend)

	s := lipgloss.JoinHorizontal(lipgloss.Top, viewStyle.Render(m.renderWorld()), viewStyle.Render(m.legendView.View()))

	return s
}

func main() {
	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		os.Exit(1)
	}
}
