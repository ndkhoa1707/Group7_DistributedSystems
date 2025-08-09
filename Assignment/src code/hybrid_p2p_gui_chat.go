// hybrid_p2p_gui_chat.go
package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/websocket"
)

type Message struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

var peerName string
var peerConns []net.Conn
var wsClients = make(map[*websocket.Conn]bool)
var wsUpgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
var broadcast = make(chan Message)

func main() {
	if len(os.Args) < 5 {
		fmt.Println("Cách dùng: go run hybrid_p2p_gui_chat.go [tên bạn] [cổng TCP] [địa chỉ TCP peer khác hoặc 'none'] [cổng WebSocket]")
		return
	}

	peerName = os.Args[1]
	listenPort := os.Args[2]
	peerAddrList := os.Args[3]
	webPort := os.Args[4]

	// Kết nối tới peer nếu có
	if peerAddrList != "none" {
		peers := strings.Split(peerAddrList, ",")
		for _, addr := range peers {
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				log.Printf("Không thể kết nối tới peer: %v", err)
				continue
			}
			log.Printf("Đã kết nối tới peer: %s", addr)
			peerConns = append(peerConns, conn)
			go handlePeerConnection(conn)
		}
	}

	// Lắng nghe các kết nối TCP đến
	go listenTCP(listenPort)

	// WebSocket handler
	http.HandleFunc("/ws", handleWS)
	http.HandleFunc("/", serveFrontend)
	http.HandleFunc("/name", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(peerName))
	})
	go handleBroadcast()

	fmt.Printf("Peer đang chạy tại: http://localhost:%s\n", webPort)
	http.ListenAndServe(":"+webPort, nil)
}

func handlePeerConnection(conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) == 2 {
			msg := Message{
				Name:    strings.TrimSpace(parts[0]),
				Message: strings.TrimSpace(parts[1]),
			}
			broadcast <- msg
		}
	}
}

func listenTCP(port string) {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatal("Lỗi listen TCP:", err)
	}
	fmt.Println("Đang lắng nghe TCP tại cổng:", port)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("Lỗi accept:", err)
			continue
		}
		peerConns = append(peerConns, conn)
		go handlePeerConnection(conn)
	}
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	ws, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket lỗi:", err)
		return
	}
	defer ws.Close()
	wsClients[ws] = true

	for {
		var msg Message
		err := ws.ReadJSON(&msg)
		if err != nil {
			log.Println("WebSocket readJSON lỗi:", err)
			delete(wsClients, ws)
			break
		}

		for _, conn := range peerConns {
			fmt.Fprintf(conn, "%s: %s\n", peerName, msg.Message)
		}

		broadcast <- msg
	}
}

func handleBroadcast() {
	for {
		msg := <-broadcast
		for client := range wsClients {
			err := client.WriteJSON(msg)
			if err != nil {
				log.Println("Gửi JSON tới frontend lỗi:", err)
				client.Close()
				delete(wsClients, client)
			}
		}
	}
}

func serveFrontend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := `
<!DOCTYPE html>
<html>
<head><title>Hybrid P2P Chat</title></head>
<body>
<h2>Hybrid P2P Chat (Group 7)</h2>
<div id="chat" style="height:300px;overflow:auto;border:1px solid #ccc;padding:10px;"></div>
<input id="message" placeholder="Nội dung tin nhắn" style="width:80%">
<button onclick="sendMsg()">Gửi</button>
<script>
let ws = new WebSocket("ws://" + location.host + "/ws");
ws.onmessage = (event) => {
  let msg = JSON.parse(event.data);
  let div = document.createElement("div");
  div.innerText = '💬 ' + msg.name + ': ' + msg.message;
  document.getElementById("chat").appendChild(div);
};
function sendMsg() {
  let content = document.getElementById("message").value;
  fetch('/name').then(r => r.text()).then(name => {
    ws.send(JSON.stringify({name: name, message: content}));
    document.getElementById("message").value = "";
  });
}
</script>
</body>
</html>`
	w.Write([]byte(html))
}
