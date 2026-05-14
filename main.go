// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-gst/go-gst/gst"
	"github.com/go-gst/go-gst/gst/app"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"

	_ "embed"
)

//go:embed home.html
var homeHTML string

// nolint: gochecknoglobals
var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	peerConnectionConfig = webrtc.Configuration{}
	settingEngine        = webrtc.SettingEngine{}

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
	httpPathPrefix := ""
	flag.StringVar(&containerPath, "container-path", "", "path to the media file you want to playback")
	flag.StringVar(&httpListenAddress, "http-listen-address", ":8080", "address for HTTP server to listen on")
	flag.StringVar(&httpPathPrefix, "http-path-prefix", "", "prefix to host this application from")
	flag.Parse()

	if containerPath == "" {
		panic("-container-path must be specified")
	}
	if httpPathPrefix != "" && (!strings.HasPrefix(httpPathPrefix, "/") || strings.HasSuffix(httpPathPrefix, "/")) {
		panic("-http-path-prefix must begin with a '/', or be empty, but must not end with a '/'")
	}

	settingEngine.SetNetworkTypes([]webrtc.NetworkType{
		webrtc.NetworkTypeTCP4,
		webrtc.NetworkTypeUDP4,
		webrtc.NetworkTypeUDP6,
	})

	tcpListener, err := net.ListenTCP("tcp4", &net.TCPAddr{
		IP:   net.IP{0, 0, 0, 0},
		Port: 8443,
	})
	if err != nil {
		panic(err)
	}

	settingEngine.SetICETCPMux(webrtc.NewICETCPMux(nil, tcpListener, 8))
	settingEngine.SetIncludeLoopbackCandidate(true)

	videoTrack, err = webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "synced") // nolint: lll
	if err != nil {
		log.Fatal(err)
	}

	audioTrack, err = webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "audio", "synced") // nolint: lll
	if err != nil {
		log.Fatal(err)
	}

	createPipeline(containerPath)

	mux := http.NewServeMux()

	mux.HandleFunc("/", serveHome)
	mux.HandleFunc("/ws", serveWs)

	var handler http.Handler = mux

	if httpPathPrefix != "" {
		// mux for the new root path
		prefixMux := http.NewServeMux()
		// handle /prefix as the home page
		prefixMux.HandleFunc(httpPathPrefix, serveHome)
		// redirect /prefix/ to /prefix
		prefixMux.Handle(fmt.Sprintf("%s/{$}", httpPathPrefix), http.RedirectHandler(httpPathPrefix, http.StatusMovedPermanently))
		// forward /prefix/... to the original mux without /prefix
		prefixMux.Handle(fmt.Sprintf("%s/", httpPathPrefix), http.StripPrefix(httpPathPrefix, mux))
		handler = prefixMux
	}

	fmt.Printf("Video file '%s' is now available on '%s%s', have fun! \n", containerPath, httpListenAddress, httpPathPrefix)
	log.Fatal(http.ListenAndServe(httpListenAddress, handler)) // nolint: gosec
}

func handleWebsocketMessage(pc *webrtc.PeerConnection, ws *websocket.Conn, message *websocketMessage) error { // nolint: cyclop,lll
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
		if err = pc.SetLocalDescription(answer); err != nil {
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

func serveWs(w http.ResponseWriter, r *http.Request) { // nolint: cyclop
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)

		return
	}

	peerConnection, err := webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine)).NewPeerConnection(peerConnectionConfig)
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

func createPipeline(containerPath string) {
	createAppSinkCallback := func(track *webrtc.TrackLocalStaticSample) *app.SinkCallbacks {
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

				if err := track.WriteSample(media.Sample{Data: samples, Duration: *buffer.Duration().AsDuration()}); err != nil {
					panic(err) //nolint
				}

				return gst.FlowOK
			},
		}
	}

	uri, err := url.Parse(containerPath)
	if err != nil {
		panic(err)
	} else if uri.Scheme == "" {
		containerPath = "file://" + containerPath
	}

	pipeline, err = gst.NewPipelineFromString(fmt.Sprintf("uridecodebin3 uri=\"%s\" name=demux ! queue ! x264enc bframes=0 speed-preset=veryfast key-int-max=60 ! video/x-h264,stream-format=byte-stream ! appsink name=video demux. ! queue ! audioconvert ! audioresample ! opusenc ! appsink name=audio", containerPath)) // nolint: lll
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
