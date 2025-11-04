<h4 align="center">Watch videos with friends using WebRTC, let your backend do the pausing and seeking.</h4>

<p align="center">
  <img style="min-width:100%;" src="https://raw.githubusercontent.com/pion/rtwatch/0d148eadb94c534cb62f39788251f057aea48adf/.github/rtwatch.gif">
</p>

<p align="center">
  <a href="https://pion.ly"><img src="https://img.shields.io/badge/pion-webrtc-gray.svg?longCache=true&colorB=brightgreen" alt="Pion webrtc"></a>
  <a href="https://pion.ly/slack"><img src="https://img.shields.io/badge/join-us%20on%20slack-gray.svg?longCache=true&logo=slack&colorB=brightgreen" alt="Slack Widget"></a>
  <br>
  <a href="https://goreportcard.com/report/github.com/pion/rtwatch"><img src="https://goreportcard.com/badge/github.com/pion/rtwatch" alt="Go Report Card"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT"></a>
</p>
<br>

Using Pion WebRTC and GStreamer you can now watch videos in real-time with your friends. Watch your favorite movie perfectly synchronized with multiple viewers. If someone pauses it pauses for everyone, and no one can and no one fast forward only their video.

*rtwatch* is different then any other solution because all state is stored on the backend. Only the current audio/video frame is being sent to the viewers, there is no way they can download/cache the videos either for future usage.

## Instructions
### Install GStreamer
#### Debian/Ubuntu
`sudo apt-get install libgstreamer1.0-dev libgstreamer-plugins-base1.0-dev gstreamer1.0-plugins-good gstreamer1.0-plugins-bad gstreamer1.0-plugins-ugly`
#### Windows MinGW64/MSYS2
`pacman -S mingw-w64-x86_64-gstreamer mingw-w64-x86_64-gst-libav mingw-w64-x86_64-gst-plugins-good mingw-w64-x86_64-gst-plugins-bad mingw-w64-x86_64-gst-plugins-ugly`

##### Error Handling

   pkg-config --cflags  -- gstreamer-1.0 gstreamer-app-1.0
   pkg-config: exec: "pkg-config": executable file not found in %PATH%

> After gstreamer installation,SET the `PATH` variable to `C:/{path to gstreamer}/bin`

   Sorry, unimplemented: 64 bit mode not compiled in

> RUN `$ g++ -v`

> CHECK Target- x86_64-w64-mingw32(Required).

  if not Download using [this](https://sourceforge.net/projects/mingw-w64/files/Toolchains%20targetting%20Win32/Personal%20Builds/mingw-builds/installer/mingw-w64-install.exe/download)


#### macOS
```
brew install gst-plugins-good pkg-config gst-plugins-bad gst-plugins-ugly
export PKG_CONFIG_PATH="/usr/local/opt/libffi/lib/pkgconfig"
```

### Download and build rtwatch
```
git clone https://github.com/pion/rtwatch.git
cd rtwatch
go build .
```

### Play your video
```
./rtwatch -container-path=/home/sean/video.mp4
> Video file '/home/sean/video.mp4' is now available on ':8080', have fun!
```

rtwatch uses [uridecodebin](https://gstreamer.freedesktop.org/documentation/playback/uridecodebin.html?gi-language=c) so you can launch it with a URI.
```
./rtwatch -container-path='https://ia800207.us.archive.org/15/items/BigBuckBunny_124/Content/big_buck_bunny_720p_surround.mp4'
```

### Watch your video with friends!
Open [http://localhost:8080](http://localhost:8080) and hit play. Open it in multiple tabs so you can see how it syncs between multiple viewers.

You also have the option to Seek/Play/Pause! Press those buttons and watch the video state change for every viewer at the same time.
