package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

func userFromAimeID(db *sql.DB, aimeCardID string) (string, error) {
	var user string
	query := "SELECT user FROM aime_card WHERE access_code = ?"
	err := db.QueryRow(query, aimeCardID).Scan(&user)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("aime card not found")
		}
		return "", err
	}
	return user, nil
}

type userNameMsg string
type totalUsersMsg map[string]int

type gameItem struct {
	gameName  string
	gameCount int
}

func fetchUserCounts(db *sql.DB) tea.Cmd {
	return func() tea.Msg {
		totalUsers := make(map[string]int)

		// Chunithm
		var chuniCount int
		err := db.QueryRow("SELECT COUNT(DISTINCT user) FROM chuni_profile_data").Scan(&chuniCount)
		if err != nil {
			log.Fatal(err)
		}
		totalUsers["Chunithm"] = chuniCount

		// Ongeki
		var ongekiCount int
		err = db.QueryRow("SELECT COUNT(DISTINCT user) FROM ongeki_profile_data").Scan(&ongekiCount)
		if err != nil {
			log.Fatal(err)
		}
		totalUsers["Ongeki"] = ongekiCount

		// MaiMai
		var maiMaiCount int
		err = db.QueryRow("SELECT COUNT(DISTINCT user) FROM mai2_profile_detail").Scan(&maiMaiCount)
		if err != nil {
			log.Fatal(err)
		}
		totalUsers["MaiMai"] = maiMaiCount

		return totalUsersMsg(totalUsers)
	}
}

// Fetches the users userName based off the user ID
func fetchUserName(db *sql.DB, game string, userID string) tea.Cmd {
	return func() tea.Msg {
		var userName string
		var query string

		switch game {
		case "Chunithm":
			query = "SELECT userName FROM chuni_profile_data WHERE user =  ?"
		case "Ongeki":
			query = "SELECT userName FROM ongeki_profile_data WHERE user = ?"
		case "MaiMai":
			query = "SELECT userName FROM mai2_profile_detail WHERE user = ?"
		default:
			return userNameMsg("Invalid game")
		}

		err := db.QueryRow(query, userID).Scan(&userName)
		if err != nil {
			if err == sql.ErrNoRows {
				return userNameMsg("User not found")
			}
			log.Fatal(err)
		}

		return userNameMsg(userName)
	}
}

func (i gameItem) FilterValue() string { return i.gameName }
func (i gameItem) Title() string       { return fmt.Sprintf("%s (%d users)", i.gameName, i.gameCount) }
func (i gameItem) Description() string { return "" }

type model struct {
	list              list.Model
	selectedGame      string
	userAimeCardInput textinput.Model
	userName          string
	totalUsers        map[string]int
	db                *sql.DB
	view              string
	games             []string
}

func initialModel(db *sql.DB) model {
	games := []string{"Chunithm", "Ongeki", "MaiMai"}

	var items []list.Item
	for _, game := range games {
		items = append(items, gameItem{gameName: game, gameCount: 0})
	}

	delegate := list.NewDefaultDelegate()

	cursor := lipgloss.NewStyle().
		Border(lipgloss.Border{
			Left: ">",
		}, false, false, false, true).
		BorderForeground(lipgloss.Color("205")).
		PaddingLeft(1).
		MarginLeft(1)

	delegate.Styles.SelectedTitle = cursor
	delegate.Styles.SelectedDesc = lipgloss.NewStyle()

	list := list.New(items, delegate, 40, 20)
	list.Title = "Select a Game"
	list.Styles.Title = lipgloss.NewStyle().
		PaddingTop(1).
		Foreground(lipgloss.Color("205")).
		Bold(true)

	list.SetFilteringEnabled(false)
	list.SetShowStatusBar(false)
	list.SetShowHelp(false)

	userIDInput := textinput.New()
	userIDInput.Placeholder = "Enter Aime Card ID"
	userIDInput.Focus()
	userIDInput.CharLimit = 32

	return model{
		list:              list,
		userAimeCardInput: userIDInput,
		totalUsers:        make(map[string]int),
		db:                db,
		view:              "gameSelection",
		games:             games,
	}
}

func (m model) Init() tea.Cmd {
	return fetchUserCounts(m.db)
}

// Update is called when messages are received. The idea is that you inspect the
// message and send back an updated model accordingly. You can also return
// a command, which is a function that performs I/O and returns a message.

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "enter":
			if m.view == "gameSelection" {
				selectedItem, ok := m.list.SelectedItem().(gameItem)
				if ok {
					m.selectedGame = selectedItem.gameName
					m.view = "aimeCardInput"
					m.userAimeCardInput.Focus()
				}
				return m, nil
			} else if m.view == "aimeCardInput" {
				aimeCardID := m.userAimeCardInput.Value()
				if aimeCardID != "" {
					user, err := userFromAimeID(m.db, aimeCardID)
					if err != nil {
						log.Printf("Error fetching user ID from Aime card: %v", err)
						return m, nil
					}
					m.userAimeCardInput.SetValue(user) // Store the user ID for later use
					return m, fetchUserName(m.db, m.selectedGame, user)
				}
			}
		case "esc": // clear everything if esc is  pressed and go to home view
			if m.view == "aimeCardInput" || m.view == "userDisplay" {
				m.view = "gameSelection"
				m.userName = ""
				m.userAimeCardInput.Reset()
				return m, nil
			}
		case "e":
			if m.view == "userDisplay" {
				userID := m.userAimeCardInput.Value() // get user value from fetchUserIDFromAimeCard() (aime card input) and use it
				switch m.selectedGame {
				case "Chunithm":
					ChunitachiExport, err := fetchChuniTachiExport(m.db, userID) // use retrevied user id from aime card
					if err != nil {
						log.Printf("Error fetching ChuniTachi export: %v", err)
						return m, nil
					}
					if err := exportChuniToTachi(ChunitachiExport); err != nil {
						log.Printf("Error exporting Chuni to Tachi: %v", err)
						return m, nil
					}
					fmt.Println("Exported to Tachi and saved to chuni_tachi_export.json")

				case "Ongeki":
					GekiTachiExport, err := fetchOngekiExport(m.db, userID)
					if err != nil {
						log.Printf("Error fetching ChuniTachi export: %v", err)
						return m, nil
					}
					if err := exportOngekiToTachi(GekiTachiExport); err != nil {
						log.Printf("Error exporting Ongeki to Tachi: %v", err)
						return m, nil
					}
					fmt.Println("Exported to Tachi and saved to ongeki_tachi_export.json")

				case "MaiMai":
					fmt.Println("MaiMai export not implemented yet")

				default:
					fmt.Printf("Unsupported game: %s\n", m.selectedGame)
					return m, nil
				}

				return m, nil
			}
		}
	case totalUsersMsg:
		m.totalUsers = msg
		var newItems []list.Item
		for _, gamename := range m.games {
			count := m.totalUsers[gamename]
			newItems = append(newItems, gameItem{gameName: gamename, gameCount: count})
		}
		m.list.SetItems(newItems)
		return m, nil
	case userNameMsg:
		m.userName = string(msg)
		m.view = "userDisplay"
		return m, nil
	}

	var cmd tea.Cmd
	if m.view == "gameSelection" {
		m.list, cmd = m.list.Update(msg)
	} else if m.view == "aimeCardInput" {
		m.userAimeCardInput, cmd = m.userAimeCardInput.Update(msg)
	}
	return m, cmd
}

// View returns a string based on data in the model. That string which will be
// rendered to the terminal.
func (m model) View() string {
	switch m.view {
	case "gameSelection":
		return m.list.View() + "\n\nPress Enter to select a game, q to quit."
	case "aimeCardInput":
		return fmt.Sprintf("Selected Game: %s\nEnter Aime Card ID: %s\nPress Enter to continue, Esc to go back.", m.selectedGame, m.userAimeCardInput.View())
	case "userDisplay":
		return fmt.Sprintf("Selected Game: %s\nUser ID: %s\nUserName: %s\nPress 'e' to export to Tachi, Esc to go back.", m.selectedGame, m.userAimeCardInput.Value(), m.userName)
	}
	return ""
}

func main() {

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		log.Fatal("DB_URL is not set in the .env file")
	}

	// Connect to MySQL
	db, err := sql.Open("mysql", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	p := tea.NewProgram(initialModel(db))
	if _, err := p.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}
