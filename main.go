package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/gorilla/websocket"
	"github.com/pion/rtwatch/gst"
	"github.com/pion/webrtc/v3"
)

const homeHTML = `<!DOCTYPE html>
<html lang="en">
	<head>
		<title>synced-playback</title>
	</head>
	<body id="body">
		<video id="video1" autoplay playsinline></video>

		<div>
		  <input type="number" id="seekTime" value="30">
		  <button type="button" onClick="seekClick()">Seek</button>
		  <button type="button" onClick="playClick()">Play</button>
		  <button type="button" onClick="pauseClick()">Pause</button>
		</div>

		<script>
			let conn = new WebSocket('ws://' + window.location.host + '/ws')
			let pc = new RTCPeerConnection()

			window.seekClick = () => {
				conn.send(JSON.stringify({event: 'seek', data: document.getElementById('seekTime').value}))
			}
			window.playClick = () => {
				conn.send(JSON.stringify({event: 'play', data: ''}))
			}
			window.pauseClick = () => {
				conn.send(JSON.stringify({event: 'pause', data: ''}))
			}

			pc.ontrack = function (event) {
			  if (event.track.kind === 'audio') {
				return
			  }
			  var el = document.getElementById('video1')
			  el.srcObject = event.streams[0]
			  el.autoplay = true
			  el.controls = true
			}

			conn.onopen = () => {
				pc.createOffer({offerToReceiveVideo: true, offerToReceiveAudio: true}).then(offer => {
					pc.setLocalDescription(offer)
					conn.send(JSON.stringify({event: 'offer', data: JSON.stringify(offer)}))
				})
			}
			conn.onclose = evt => {
				console.log('Connection closed')
			}
			conn.onmessage = evt => {
				let msg = JSON.parse(evt.data)
				if (!msg) {
					return console.log('failed to parse msg')
				}

				switch (msg.event) {
				case 'answer':
					answer = JSON.parse(msg.data)
					if (!answer) {
						return console.log('failed to parse answer')
					}
					pc.setRemoteDescription(answer)
				}
			}
			window.conn = conn
		</script>
	</body>
</html>
`

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	peerConnectionConfig = webrtc.Configuration{}

	audioTrack = &webrtc.TrackLocalStaticSample{}
	videoTrack = &webrtc.TrackLocalStaticSample{}
	pipeline   = &gst.Pipeline{}
)

type websocketMessage struct {
	Event string `json:"event"`
	Data  string `json:"data"`
}

func main() {
	containerPath := ""
	httpListenAddress := ""
	flag.StringVar(&containerPath, "container-path", "", "path to the media file you want to playback")
	flag.StringVar(&httpListenAddress, "http-listen-address", ":8080", "address for HTTP server to listen on")
	flag.Parse()

	if containerPath == "" {
		panic("-container-path must be specified")
	}

	var err error
	videoTrack, err = webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: "video/h264"}, "synced", "video")
	if err != nil {
		log.Fatal(err)
	}

	audioTrack, err = webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: "audio/opus"}, "synced", "audio")
	if err != nil {
		log.Fatal(err)
	}

	pipeline = gst.CreatePipeline(containerPath, audioTrack, videoTrack)
	pipeline.Start()

	http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", serveWs)

	fmt.Printf("Video file '%s' is now available on '%s', have fun! \n", containerPath, httpListenAddress)
	log.Fatal(http.ListenAndServe(httpListenAddress, nil))
}

func handleWebsocketMessage(pc *webrtc.PeerConnection, ws *websocket.Conn, message *websocketMessage) error {
	switch message.Event {
	case "play":
		pipeline.Play()
	case "pause":
		pipeline.Pause()
	case "seek":
		i, err := strconv.ParseInt(message.Data, 0, 64)
		if err != nil {
			log.Print(err)
		}
		pipeline.SeekToTime(i)
	case "offer":
		offer := webrtc.SessionDescription{}
		if err := json.Unmarshal([]byte(message.Data), &offer); err != nil {
			return err
		}

		if err := pc.SetRemoteDescription(offer); err != nil {
			return err
		}

		answer, err := pc.CreateAnswer(nil)
		if err != nil {
			return err
		}

		gatherComplete := webrtc.GatheringCompletePromise(pc)

		if err := pc.SetLocalDescription(answer); err != nil {
			return err
		}

		<-gatherComplete

		answerString, err := json.Marshal(pc.LocalDescription())
		if err != nil {
			return err
		}

		if err = ws.WriteJSON(&websocketMessage{
			Event: "answer",
			Data:  string(answerString),
		}); err != nil {
			return err
		}
	}
	return nil
}

func serveWs(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		if _, ok := err.(websocket.HandshakeError); !ok {
			log.Println(err)
		}
		return
	}

	peerConnection, err := webrtc.NewPeerConnection(peerConnectionConfig)
	if err != nil {
		log.Print(err)
		return
	} else if _, err = peerConnection.AddTrack(audioTrack); err != nil {
		log.Print(err)
		return
	} else if _, err = peerConnection.AddTrack(videoTrack); err != nil {
		log.Print(err)
		return
	}

	defer func() {
		if err := peerConnection.Close(); err != nil {
			log.Println(err)
		}
	}()

	message := &websocketMessage{}
	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			break
		} else if err := json.Unmarshal(msg, &message); err != nil {
			log.Print(err)
			return
		}

		if err := handleWebsocketMessage(peerConnection, ws, message); err != nil {
			log.Print(err)
		}
	}
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, homeHTML)
}
