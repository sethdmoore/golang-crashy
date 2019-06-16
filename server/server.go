package server

import (
	"bitbucket.org/hypomanic/carddemoserver/gamestate"
	"bitbucket.org/hypomanic/carddemoserver/cardstub"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/golang/protobuf/proto"
	"github.com/gorilla/websocket"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"time"
)

type connectedUser struct {
	sessionID  string
	username   string
	connection websocket.Conn
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type session struct {
	SessionID string `json:"session-id"`
}

var upgrader = websocket.Upgrader{}

var connectedClients = make(map[*websocket.Conn]bool)

var sendMessage = make(chan string)
var sendGameState = make(chan []byte)
var connectedUsers = make(map[string]*connectedUser)

func NewUser(sid string, username string) *connectedUser {
	return &connectedUser{sessionID: sid, username: username}
}

var gs gamestate.GameState

func loginHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var creds credentials
	var sess session

	if r.Method != "POST" {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		fmt.Printf("Invalid request method: %s\n", r.Method)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Internal Server Error",
			http.StatusInternalServerError)
		fmt.Printf("Error reading request body: %v\n", err)
		return
	}

	// spew.Dump(body)

	err = json.Unmarshal(body, &creds)
	if err != nil {
		http.Error(w, "Internal Server Error",
			http.StatusInternalServerError)
		fmt.Printf("Error parsing body as JSON: %v\n", err)
		return
	}

	sess.SessionID = createID()
	result, err := json.Marshal(&sess)
	if err != nil {
		http.Error(w, "Internal Server Error",
			http.StatusInternalServerError)
		fmt.Printf("Error marshalling SessionID: %v\n", err)
		return
	}
	_, err = w.Write(result)
	if err != nil {
		fmt.Printf("ERROR: writing session ID back: %v\n", err)
	}

	connectedUsers[sess.SessionID] = NewUser(sess.SessionID, creds.Username)

	fmt.Printf("Sending session ID client: %v\n", sess.SessionID)
}

func createID() string {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(b)
}

func chatHandler(w http.ResponseWriter, r *http.Request) {
	var err error

	var connected bool = false
	var username string

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Printf("ERROR: could not HTTP UPGRADE for ws: %v\n", err)
		return
	}

	defer ws.Close()

	connectedClients[ws] = true

	fmt.Println("Chat connection")

	for {
		mt, wsMsg, err := ws.ReadMessage()
		if err != nil {
			fmt.Printf("Chat Disconnected: %v\n", err)
			spew.Dump(wsMsg)
			delete(connectedClients, ws)
			break
		}
		// determine if message was []byte or string
		switch mt {
		case websocket.TextMessage:
			msg := string(wsMsg)
			if connected == false {
				fmt.Println("not connected")
				//I need error handling or I need to see if the mapping exists before calling this.
				if val, ok := connectedUsers[msg]; ok {
					if val.sessionID == msg {
						connectedUsers[msg].connection = *ws
						fmt.Println("connected")
						connected = true
						username = connectedUsers[msg].username
						msg = val.sessionID
					}
				}
			} else {
				msg = username + ": " + msg
				// spew.Dump(msg)
				fmt.Println("Sending to client: " + msg)
				sendMessage <- msg
			}
		case websocket.BinaryMessage:
			//msg := wsMsg.([]byte)
			fmt.Printf("INFO: Received bytes")
			sendGameState <- wsMsg
		default:
			fmt.Printf("ERROR: other value returned from websocket: %v", reflect.TypeOf(wsMsg))
			spew.Dump(wsMsg)

		}

	}
}

func broadcastMessagesToClients() {
	for {
		msg := <-sendMessage

		for client := range connectedClients {
			err := client.WriteMessage(websocket.TextMessage, []byte(msg))
			if err != nil {
				fmt.Printf("ERROR: %v", err)
				client.Close()
				delete(connectedClients, client)
			}
		}
	}
}

func broadcastGamestateToClients() {
	for {
		bin := <-sendGameState

		for client := range connectedClients {
			err := client.WriteMessage(websocket.BinaryMessage, bin)
			if err != nil {
				fmt.Printf("ERROR: %v", err)
				client.Close()
				delete(connectedClients, client)
			}
		}
	}
}

func sendDebugData() {
	var counter int

	gs := &gamestate.GameState{
		SessionID: "Floof",
		Players: []*gamestate.Players{
			&gamestate.Players{
				Deck: &gamestate.Deck{
					Cards: []*gamestate.Card{
						&gamestate.Card{
							Name:        "Foo",
							Description: "Bringer of Foos",
						},
					},
				},
			},
		},
	}
	gs.Players[0].Deck = cardstub.DeckGenerator()
	//gs.Players = make([])

	for {
		time.Sleep(2 * time.Second)
		counter++
		gs.SessionID = fmt.Sprintf("FLUFFY CATS %d", counter)

		data, err := proto.Marshal(gs)
		if err != nil {
			fmt.Printf("ERROR: could not marshal protobuf: %v\n", err)
			continue
		}
		fmt.Printf("INFO: sending stuff?\n")
		sendGameState <- data
	}
}

func Start(host string, port string) error {
	http.HandleFunc("/login", loginHandler)

	http.HandleFunc("/chat", chatHandler)

	go broadcastMessagesToClients()
	go broadcastGamestateToClients()
	// XXX: debug: inject gamestate
	go sendDebugData()

	fmt.Printf("Listening on %s\n", host+port)
	err := http.ListenAndServe(port, nil)
	if err != nil {
		return err
	}
	return nil
}
