package main

import (
	"fmt"
	"github.com/gorilla/websocket"
	"net/http"
)

var connectedClients = make(map[*websocket.Conn]bool)
var sendMessage = make(chan string)

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

func connHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	var upgrader = websocket.Upgrader{}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Printf("ERROR: could not HTTP UPGRADE for ws: %v\n", err)
		return
	}

	defer ws.Close()

	// connection map
	connectedClients[ws] = true

	for {
		mt, wsMsg, err := ws.ReadMessage()
		if err != nil {
			fmt.Printf("Chat Disconnected: %v\n", err)
			delete(connectedClients, ws)
			break
		}
		// determine if message was []byte or string
		switch mt {
		case websocket.TextMessage:
			msg := string(wsMsg)
			sendMessage <- msg
		default:
			fmt.Printf("ERROR: non string value received\n")

		}

	}
}

func main() {
	fmt.Println("Server Started.")
	host := "0.0.0.0"
	port := "4444"

	go broadcastMessagesToClients()

	http.HandleFunc("/", connHandler)

	err := http.ListenAndServe(host+":"+port, nil)
	if err != nil {
		panic("ListenAndServe: " + err.Error())
	}
}
