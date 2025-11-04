package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/go-gst/go-gst/gst/app"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

const homeHTML = `<!DOCTYPE html>
<html lang="en">
	<head>
		<title>rtwatch</title>
	</head>
	<body id="body">
		<video id="video1" autoplay playsinline controls></video>

		<div>
		  <input type="number" id="seekTime" value="30">
		  <button type="button" onClick="seekClick()">Seek</button>
		  <button type="button" onClick="playClick()">Play</button>
		  <button type="button" onClick="pauseClick()">Pause</button>
		</div>

		<div>
		  Connection State: <span id="connectionState"> </span>
		</div>

		<script>
			let conn = new WebSocket('ws://' + window.location.host + '/ws')
			let pc = new RTCPeerConnection()

			pc.onconnectionstatechange = () => {
				document.getElementById('connectionState').innerText = pc.connectionState
			}

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
			  var el = document.getElementById('video1')
			  el.srcObject = event.streams[0]
			}

			conn.onopen = () => {
				pc.addTransceiver('audio', { direction: 'recvonly' });
				pc.addTransceiver('video', { direction: 'recvonly' });
				pc.createOffer().then(offer => {
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
	// Initialize GStreamer
	gst.Init(nil)

	containerPath := ""
	httpListenAddress := ""
	flag.StringVar(&containerPath, "container-path", "", "path to the media file you want to playback")
	flag.StringVar(&httpListenAddress, "http-listen-address", ":8080", "address for HTTP server to listen on")
	flag.Parse()

	if containerPath == "" {
		panic("-container-path must be specified")
	}

	var err error
	videoTrack, err = webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "synced")
	if err != nil {
		log.Fatal(err)
	}

	audioTrack, err = webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "synced")
	if err != nil {
		log.Fatal(err)
	}

	createPipeline(containerPath)

	http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", serveWs)

	fmt.Printf("Video file '%s' is now available on '%s', have fun! \n", containerPath, httpListenAddress)
	log.Fatal(http.ListenAndServe(httpListenAddress, nil))
}

func handleWebsocketMessage(pc *webrtc.PeerConnection, ws *websocket.Conn, message *websocketMessage) error {
	switch message.Event {
	case "play":
		if err := pipeline.SetState(gst.StatePlaying); err != nil {
			log.Print(err)
		}
	case "pause":
		if err := pipeline.SetState(gst.StatePaused); err != nil {
			log.Print(err)
		}
	case "seek":
		i, err := strconv.ParseInt(message.Data, 0, 64)
		if err != nil {
			log.Print(err)
		}
		pipeline.SeekTime(time.Duration(i)*time.Second, gst.SeekFlagFlush|gst.SeekFlagKeyUnit|gst.SeekFlagSkip)
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
	_, _ = fmt.Fprint(w, homeHTML)
}

func createPipeline(containerPath string) {
	createAppSinkCallback := func(t *webrtc.TrackLocalStaticSample) *app.SinkCallbacks {
		return &app.SinkCallbacks{
			NewSampleFunc: func(sink *app.Sink) gst.FlowReturn {
				sample := sink.PullSample()
				if sample == nil {
					return gst.FlowEOS
				}

				buffer := sample.GetBuffer()
				if buffer == nil {
					return gst.FlowError
				}

				samples := buffer.Map(gst.MapRead).Bytes()
				defer buffer.Unmap()

				if err := t.WriteSample(media.Sample{Data: samples, Duration: *buffer.Duration().AsDuration()}); err != nil {
					panic(err) //nolint
				}

				return gst.FlowOK
			},
		}
	}

	var err error
	pipeline, err = gst.NewPipelineFromString(fmt.Sprintf("filesrc location=\"%s\" ! decodebin name=demux ! queue ! x264enc bframes=0 speed-preset=veryfast key-int-max=60 ! video/x-h264,stream-format=byte-stream ! appsink name=video demux. ! queue ! audioconvert ! audioresample ! opusenc ! appsink name=audio", containerPath))
	if err != nil {
		panic(err)
	}

	if err = pipeline.SetState(gst.StatePlaying); err != nil {
		panic(err)
	}

	audioSink, err := pipeline.GetElementByName("audio")
	if err != nil {
		panic(err)
	}

	videoSink, err := pipeline.GetElementByName("video")
	if err != nil {
		panic(err)
	}

	app.SinkFromElement(audioSink).SetCallbacks(createAppSinkCallback(audioTrack))
	app.SinkFromElement(videoSink).SetCallbacks(createAppSinkCallback(videoTrack))
}
