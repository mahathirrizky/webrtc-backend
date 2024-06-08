package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

// WebSocket upgrader to handle HTTP to WebSocket conversion
var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	listLock sync.RWMutex // RWMutex to synchronize access to rooms
	rooms    = make(map[string]*Room) // Map to store rooms
)

// Struct to define the format of WebSocket messages
type websocketMessage struct {
	Event  string `json:"event"`
	Data   string `json:"data"`
	RoomID string `json:"roomId"`
}

// Struct to hold peer connection state
type peerConnectionState struct {
	peerConnection *webrtc.PeerConnection
	websocket      *threadSafeWriter
}

// Struct to define a room
type Room struct {
	peerConnections []peerConnectionState
	trackLocals     map[string]*webrtc.TrackLocalStaticRTP
}

// Function to get a room, create one if it doesn't exist
func getRoom(roomId string) *Room {
	listLock.Lock()
	defer listLock.Unlock()
	if room, ok := rooms[roomId]; ok {
		return room
	}
	room := &Room{
		peerConnections: []peerConnectionState{},
		trackLocals:     make(map[string]*webrtc.TrackLocalStaticRTP),
	}
	rooms[roomId] = room
	return room
}

// Function to add a track to a room
func addTrack(room *Room, t *webrtc.TrackRemote) *webrtc.TrackLocalStaticRTP {
	listLock.Lock()
	defer func() {
		listLock.Unlock()
		signalPeerConnections(room)
	}()

	// Create a local track to send RTP
	trackLocal, err := webrtc.NewTrackLocalStaticRTP(t.Codec().RTPCodecCapability, t.ID(), t.StreamID())
	if err != nil {
		panic(err)
	}

	room.trackLocals[t.ID()] = trackLocal
	return trackLocal
}

// Function to remove a track from a room
func removeTrack(room *Room, t *webrtc.TrackLocalStaticRTP) {
	listLock.Lock()
	defer func() {
		listLock.Unlock()
		signalPeerConnections(room)
	}()

	delete(room.trackLocals, t.ID())
}

// Function to signal all peer connections in a room
func signalPeerConnections(room *Room) {
	listLock.Lock()
	defer func() {
		listLock.Unlock()
		DispatchKeyFrame()
	}()

	// Attempt to sync all peer connections
	attemptSync := func() (tryAgain bool) {
		for i := range room.peerConnections {
			if room.peerConnections[i].peerConnection.ConnectionState() == webrtc.PeerConnectionStateClosed {
				room.peerConnections = append(room.peerConnections[:i], room.peerConnections[i+1:]...)
				return true
			}

			existingSenders := map[string]bool{}

			// Remove tracks that are no longer available
			for _, sender := range room.peerConnections[i].peerConnection.GetSenders() {
				if sender.Track() == nil {
					continue
				}

				existingSenders[sender.Track().ID()] = true

				if _, ok := room.trackLocals[sender.Track().ID()]; !ok {
					if err := room.peerConnections[i].peerConnection.RemoveTrack(sender); err != nil {
						return true
					}
				}
			}

			// Add new tracks to peer connections
			for _, receiver := range room.peerConnections[i].peerConnection.GetReceivers() {
				if receiver.Track() == nil {
					continue
				}

				existingSenders[receiver.Track().ID()] = true
			}

			for trackID := range room.trackLocals {
				if _, ok := existingSenders[trackID]; !ok {
					if _, err := room.peerConnections[i].peerConnection.AddTrack(room.trackLocals[trackID]); err != nil {
						return true
					}
				}
			}

			// Create and send new offer
			offer, err := room.peerConnections[i].peerConnection.CreateOffer(nil)
			if err != nil {
				return true
			}

			if err = room.peerConnections[i].peerConnection.SetLocalDescription(offer); err != nil {
				return true
			}

			offerString, err := json.Marshal(offer)
			if err != nil {
				return true
			}

			if err = room.peerConnections[i].websocket.WriteJSON(&websocketMessage{
				Event: "offer",
				Data:  string(offerString),
			}); err != nil {
				return true
			}
		}

		return
	}

	// Retry syncing peer connections up to 25 times
	for syncAttempt := 0; ; syncAttempt++ {
		if syncAttempt == 25 {
			go func() {
				time.Sleep(time.Second * 3)
				signalPeerConnections(room)
			}()
			return
		}

		if !attemptSync() {
			break
		}
	}
}

// Function to dispatch key frames to all peer connections
func DispatchKeyFrame() {
	listLock.Lock()
	defer listLock.Unlock()

	for _, room := range rooms {
		for i := range room.peerConnections {
			for _, receiver := range room.peerConnections[i].peerConnection.GetReceivers() {
				if receiver.Track() == nil {
					continue
				}

				_ = room.peerConnections[i].peerConnection.WriteRTCP([]rtcp.Packet{
					&rtcp.PictureLossIndication{
						MediaSSRC: uint32(receiver.Track().SSRC()),
					},
				})
			}
		}
	}
}

// WebSocket handler to manage new WebSocket connections
func WebsocketHandler(w http.ResponseWriter, r *http.Request, roomId string) {
	room := getRoom(roomId)

	unsafeConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}

	c := &threadSafeWriter{unsafeConn, sync.Mutex{}}

	defer c.Close()

	peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		log.Print(err)
		return
	}

	defer peerConnection.Close()

	// Add transceivers for audio and video
	for _, typ := range []webrtc.RTPCodecType{webrtc.RTPCodecTypeVideo, webrtc.RTPCodecTypeAudio} {
		if _, err := peerConnection.AddTransceiverFromKind(typ, webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionRecvonly,
		}); err != nil {
			log.Print(err)
			return
		}
	}

	// Add peer connection to room
	listLock.Lock()
	room.peerConnections = append(room.peerConnections, peerConnectionState{peerConnection, c})
	listLock.Unlock()

	// Handle ICE candidates
	peerConnection.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i == nil {
			return
		}

		candidateString, err := json.Marshal(i.ToJSON())
		if err != nil {
			log.Println(err)
			return
		}

		if writeErr := c.WriteJSON(&websocketMessage{
			Event:  "candidate",
			Data:   string(candidateString),
			RoomID: roomId,
		}); writeErr != nil {
			log.Println(writeErr)
		}
	})

	// Handle connection state changes
	peerConnection.OnConnectionStateChange(func(p webrtc.PeerConnectionState) {
		switch p {
		case webrtc.PeerConnectionStateFailed:
			if err := peerConnection.Close(); err != nil {
				log.Print(err)
			}
		case webrtc.PeerConnectionStateClosed:
			signalPeerConnections(room)
		default:
		}
	})

	// Handle incoming tracks
	peerConnection.OnTrack(func(t *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		trackLocal := addTrack(room, t)
		defer removeTrack(room, trackLocal)

		buf := make([]byte, 1500)
		for {
			i, _, err := t.Read(buf)
			if err != nil {
				return
			}

			if _, err = trackLocal.Write(buf[:i]); err != nil {
				return
			}
		}
	})

	signalPeerConnections(room)

	message := &websocketMessage{}
	for {
		_, raw, err := c.ReadMessage()
		if err != nil {
			log.Println(err)
			return
		} else if err := json.Unmarshal(raw, &message); err != nil {
			log.Println(err)
			return
		}

		switch message.Event {
		case "candidate":
			candidate := webrtc.ICECandidateInit{}
			if err := json.Unmarshal([]byte(message.Data), &candidate); err != nil {
				log.Println(err)
				return
			}

			if err := peerConnection.AddICECandidate(candidate); err != nil {
				log.Println(err)
				return
			}
		case "answer":
			answer := webrtc.SessionDescription{}
			if err := json.Unmarshal([]byte(message.Data), &answer); err != nil {
				log.Println(err)
				return
			}

			if err := peerConnection.SetRemoteDescription(answer); err != nil {
				log.Println(err)
				return
			}
		}
	}
}

// Struct to handle thread-safe WebSocket writing
type threadSafeWriter struct {
	*websocket.Conn
	sync.Mutex
}

func (t *threadSafeWriter) WriteJSON(v interface{}) error {
	t.Lock()
	defer t.Unlock()

	return t.Conn.WriteJSON(v)
}
