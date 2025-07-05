package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/olahol/melody"
)

type Move struct {
	Player string `json:"player"`
	Cell   int    `json:"cell"`
}

type WebSocketMessage struct {
	Type   string `json:"type"`
	Player string `json:"player,omitempty"`
	Cell   int    `json:"cell,omitempty"`
}

type Game struct {
	Board       [9]string `json:"board"`
	Turn        string    `json:"turn"`
	Winner      string    `json:"winner,omitempty"`
	IsDraw      bool      `json:"is_draw,omitempty"`
	Player1     string    `json:"player1,omitempty"`
	Player2     string    `json:"player2,omitempty"`
	Player1Name string    `json:"player1_name,omitempty"`
	Player2Name string    `json:"player2_name,omitempty"`
}

var (
	m       = melody.New()
	games   = make(map[string]*Game)
	players = make(map[string]map[*melody.Session]string)
	mu      sync.Mutex
)

func main() {
	r := chi.NewRouter()
	r.Use(middleware.Logger)

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath := filepath.Dir(ex)

	// Serve the lobby page
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.ParseFiles(filepath.Join(exPath, "..", "src", "index.html"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, nil)
	})

	// Handle game creation
	r.Post("/create", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		name := r.FormValue("name")
		if name == "" {
			http.Error(w, "Name is required", http.StatusBadRequest)
			return
		}

		gameID := generateID()
		games[gameID] = &Game{Turn: "X", Player1Name: name}
		players[gameID] = make(map[*melody.Session]string)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"gameID": gameID})
	})

	// Handle joining a game
	r.Post("/join", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		name := r.FormValue("name")
		gameID := r.FormValue("gameID")

		if name == "" || gameID == "" {
			http.Error(w, "Name and Game ID are required", http.StatusBadRequest)
			return
		}

		if _, ok := games[gameID]; !ok {
			http.Error(w, "Game not found", http.StatusNotFound)
			return
		}

		if len(players[gameID]) >= 2 {
			http.Error(w, "Game is full", http.StatusConflict)
			return
		}

		games[gameID].Player2Name = name
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"gameID": gameID})
	})

	// Serve the game page
	r.Get("/game/{gameID}", func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.ParseFiles(filepath.Join(exPath, "..", "src", "game.html"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, nil)
	})

	// Handle WebSocket connections
	r.Get("/ws/{gameID}", func(w http.ResponseWriter, r *http.Request) {
		m.HandleRequest(w, r)
	})

	m.HandleConnect(handleConnect)
	m.HandleMessage(handleMessage)
	m.HandleDisconnect(handleDisconnect)

	log.Println("Server started on :3000")
	http.ListenAndServe(":3000", r)
}

func handleConnect(s *melody.Session) {
	mu.Lock()
	defer mu.Unlock()

	gameID := chi.URLParam(s.Request, "gameID")
	if _, ok := games[gameID]; !ok {
		s.Close()
		return
	}

	if len(players[gameID]) >= 2 {
		s.Close()
		return
	}

	var playerSymbol string
	if len(players[gameID]) == 0 {
		playerSymbol = "X"
		games[gameID].Player1 = playerSymbol
	} else {
		playerSymbol = "O"
		games[gameID].Player2 = playerSymbol
	}
	players[gameID][s] = playerSymbol
	s.Set("player", playerSymbol)
	s.Set("gameID", gameID)

	broadcastGameState(gameID)
}

func handleMessage(s *melody.Session, msg []byte) {
	mu.Lock()
	defer mu.Unlock()

	gameID, _ := s.Get("gameID")
	game := games[gameID.(string)]

	var message WebSocketMessage
	if err := json.Unmarshal(msg, &message); err != nil {
		return
	}

	switch message.Type {
	case "move":
		player, _ := s.Get("player")
		if player != game.Turn || game.Board[message.Cell] != "" || (game.Winner != "" || game.IsDraw) {
			return
		}

		game.Board[message.Cell] = game.Turn
		if checkWin(game, game.Turn) {
			game.Winner = game.Turn
		} else if checkDraw(game) {
			game.IsDraw = true
		} else {
			if game.Turn == "X" {
				game.Turn = "O"
			} else {
				game.Turn = "X"
			}
		}
	case "reset":
		resetGame(gameID.(string))
	case "leave_game": // New case for when a player explicitly leaves
		playerSymbol, _ := s.Get("player")
		if playerSymbol == "X" { // Host (Player 1) is leaving
			// Redirect all players in this game to the lobby
			m.BroadcastFilter([]byte(`{"type": "redirect", "url": "/"}`), func(q *melody.Session) bool {
				qGameID, _ := q.Get("gameID")
				return qGameID == gameID
			})
			// Clean up the game
			delete(games, gameID.(string))
			delete(players, gameID.(string))
		} else { // Player 2 is leaving
			resetGame(gameID.(string))
		}
	}

	// Only broadcast if the game still exists after potential deletion
	if _, ok := games[gameID.(string)]; ok {
		broadcastGameState(gameID.(string))
	}
}

func handleDisconnect(s *melody.Session) {
	mu.Lock()
	defer mu.Unlock()

	gameID, ok := s.Get("gameID")
	if !ok {
		return // Session not associated with a game
	}

	playerSymbol, _ := s.Get("player")

	// Remove the disconnected session from the game's players map
	delete(players[gameID.(string)], s)

	// Check if the game still exists and has players
	if _, gameExists := games[gameID.(string)]; gameExists {
		if len(players[gameID.(string)]) == 0 {
			// No players left in the game, delete the game
			delete(games, gameID.(string))
			delete(players, gameID.(string))
		} else {
			// One player left
			if playerSymbol == "X" { // Player 1 disconnected
				// Redirect remaining player (Player 2) to lobby
				m.BroadcastFilter([]byte(`{"type": "redirect", "url": "/"}`), func(q *melody.Session) bool {
					qGameID, _ := q.Get("gameID")
					return qGameID == gameID
				})
				// Delete the game
				delete(games, gameID.(string))
				delete(players, gameID.(string))
			} else { // Player 2 disconnected
				// Reset the game for Player 1
				resetGame(gameID.(string))
				broadcastGameState(gameID.(string))
			}
		}
	}
}

func broadcastGameState(gameID string) {
	msg, _ := json.Marshal(games[gameID])
	m.BroadcastFilter(msg, func(q *melody.Session) bool {
		qGameID, _ := q.Get("gameID")
		return qGameID == gameID
	})
}

func resetGame(gameID string) {
	games[gameID].Board = [9]string{}
	games[gameID].Winner = ""
	games[gameID].IsDraw = false
	games[gameID].Turn = "X"

	// Clear Player 2's data if they were the one who left
	games[gameID].Player2 = ""
	games[gameID].Player2Name = ""
}

func checkWin(game *Game, player string) bool {
	winConditions := [][3]int{
		{0, 1, 2}, {3, 4, 5}, {6, 7, 8},
		{0, 3, 6}, {1, 4, 7}, {2, 5, 8},
		{0, 4, 8}, {2, 4, 6},
	}
	for _, wc := range winConditions {
		if game.Board[wc[0]] == player && game.Board[wc[1]] == player && game.Board[wc[2]] == player {
			return true
		}
	}
	return false
}

func checkDraw(game *Game) bool {
	for _, cell := range game.Board {
		if cell == "" {
			return false
		}
	}
	return true
}

func generateID() string {
	bytes := make([]byte, 3) // 6 characters long
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
