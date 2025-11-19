package main

import (
	"log"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Maximum message size allowed from peer.
	maxMessageSize = 2048
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type BroadcastMessage struct {
	senderID byte
	data     []byte
}

type Client struct {
	hub *Hub
	conn *websocket.Conn
	id byte
	send chan []byte
}

// reader is the dedicated goroutine for reading messages from the WebSocket connection.
func (c *Client) reader() {
	defer func() {
		// When the reader exits (due to error or close), signal the hub to clean up.
		log.Printf("Reader for ID %d exiting.", c.id)
		c.hub.unregister <- c.id
	}()

	c.conn.SetReadLimit(maxMessageSize)

	for {
		messageType, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Client %d read error: %v", c.id, err)
			}
			break
		}

		if messageType == websocket.BinaryMessage {
			if len(message) > maxMessageSize {
				message = message[:maxMessageSize]
				log.Printf("Client %d message truncated to %d bytes.", c.id, maxMessageSize)
			}

			//log.Printf("Client %d received binary message (Size: %d bytes)", c.id, len(message))
			c.hub.broadcast <- BroadcastMessage{
				senderID: c.id,
				data:     message,
			}
		} else {
			log.Printf("Client %d sent non-binary message type (%d), ignoring.", c.id, messageType)
		}
	}
}

// Writer goroutine per-client connection
func (c *Client) writer() {
	defer func() {
		log.Printf("Writer for ID %d exiting.", c.id)
		c.hub.unregister <- c.id
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))

			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.BinaryMessage, message); err != nil {
				log.Printf("Writer for ID %d failed to write: %v", c.id, err)
				return
			}
		}
	}
}

// Hub maintains the set of active clients and broadcasts messages to them.
type Hub struct {
	clients map[byte]*Client
	mu      sync.RWMutex
	broadcast chan BroadcastMessage
	register chan *Client
	unregister chan byte
	nextID atomic.Uint32
}

// NewHub creates and returns a new Hub instance.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[byte]*Client),
		broadcast:  make(chan BroadcastMessage),
		register:   make(chan *Client),
		unregister: make(chan byte),
	}
}

// generateID generates a unique 1-byte ID for a new client.
func (h *Hub) generateID() byte {
	// Atomically increment the counter and mask it to ensure it fits in a byte (0-255)
	return byte(h.nextID.Add(1) % 256)
}

// run starts the hub's main goroutine, handling registration, unregistration, and broadcasting.
func (h *Hub) run() {
	log.Println("Hub started, listening for connections and messages.")
	for {
		select {
		case client := <-h.register:
			// 1. Register a new client
			h.mu.Lock()
			h.clients[client.id] = client
			h.mu.Unlock()
			log.Printf("Client registered. ID: %d, Address: %s. Total clients: %d",
				client.id, client.conn.RemoteAddr(), len(h.clients))

			// Send the client its own ID via the dedicated channel
			client.send <- []byte{client.id}

			// Start the reader and the new, dedicated writer goroutines
			go client.writer()
			go client.reader()

		case clientID := <-h.unregister:
			// 2. Unregister and clean up a client
			h.mu.Lock()
			if client, ok := h.clients[clientID]; ok {
				delete(h.clients, clientID)
				close(client.send)
				client.conn.Close()
				log.Printf("Client unregistered (ID: %d). Total clients: %d", clientID, len(h.clients))
			}
			h.mu.Unlock()

		case msg := <-h.broadcast:
			// 3. Broadcast a message
			//log.Printf("Broadcasting message from ID %d (Size: %d bytes)", msg.senderID, len(msg.data))
			fullMsg := append([]byte{msg.senderID}, msg.data...)
			h.mu.RLock()
			for id, client := range h.clients {
				// Send to all clients EXCEPT the sender
				if id != msg.senderID {
					select {
					case client.send <- fullMsg:
						// Message queued successfully
					default:
						// If the send channel is full, the client is probably slow or dead.
						log.Printf("Client %d send channel full, unregistering.", id)
						close(client.send)
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}

	clientID := hub.generateID()
	client := &Client{
		hub:  hub,
		conn: conn,
		id:   clientID,
		send: make(chan []byte, 256),
	}

	// On successful upgrade, register the client struct with the hub.
	hub.register <- client
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// Create and start the Hub
	hub := NewHub()
	go hub.run()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if websocket.IsWebSocketUpgrade(r) {
			serveWs(hub, w, r)
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("WebSocket Echo Server is running. Connect via WebSocket to ws://localhost:8765/"))
		}
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8765"
	}
	addr := ":" + port

	log.Printf("Starting server on http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
