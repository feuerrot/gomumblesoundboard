package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	_ "github.com/feuerrot/gomumblesoundboard/opus"
	"layeh.com/gumble/gumble"
	"layeh.com/gumble/gumbleffmpeg"
	"layeh.com/gumble/gumbleutil"
)

type Interaction struct {
	Stop   bool
	Volume float32
	File   *File
}

var (
	targetChannel = flag.String("channel", "Root", "channel the bot will join")
	maxVolume     = flag.String("maxvol", "100", "Set the maximum Volume in %, the volume set in the UI is multiplied with it")
)

func main() {
	s := sb{
		iChan: make(chan Interaction),
	}

	gumbleutil.Main(
		gumbleutil.AutoBitrate,

		// Parse volume parameter
		OnStartListener(func() {
			maxVolumeF, err := strconv.Atoi(*maxVolume)
			if err != nil {
				fmt.Printf("Invalid MaxVolume %d", maxVolumeF)
				os.Exit(1)
			}

			s.maxVol = float32(maxVolumeF) / 100
			s.currVol = s.maxVol
			fmt.Printf("maximum Volume: %.1f%%\n", s.maxVol*100)
		}),

		// Scan for Files
		OnStartListener(func() {
			scanDirs(flag.Args())
			fmt.Printf("GoMumbleSoundboard loaded (%d files)\n", len(soundfiles))
		}),

		// Setup HTTP routes and listener
		OnStartListener(s.setupHTTP),

		gumbleutil.Listener{
			Connect: func(e *gumble.ConnectEvent) {
				stream := gumbleffmpeg.New(e.Client, nil)
				stream.Volume = 1

				e.Client.Self.SetSelfDeafened(true)

				fmt.Printf("Connected to %s\n", e.Client.Conn.RemoteAddr())
				fmt.Printf("Current Channel: %s\n", e.Client.Self.Channel.Name)

				if *targetChannel != "" && e.Client.Self.Channel.Name != *targetChannel {
					channelPath := strings.Split(*targetChannel, "/")
					target := e.Client.Self.Channel.Find(channelPath...)
					if target == nil {
						fmt.Printf("Cannot find channel named %s\n", *targetChannel)
						os.Exit(1)
					}
					e.Client.Self.Move(target)
					fmt.Printf("Moved to: %s\n", target.Name)
				}

				for interaction := range s.iChan {
					if interaction.Stop == true {
						_ = stream.Stop()
					}

					if interaction.Volume != 0 {
						s.currVol = interaction.Volume
						stream.Volume = s.currVol
					}

					if interaction.File != nil {
						e.Client.Self.SetSelfMuted(false)
						stream = gumbleffmpeg.New(e.Client, gumbleffmpeg.SourceFile(interaction.File.FullPath))
						stream.Volume = s.currVol

						if err := stream.Play(); err != nil {
							return
						}

						go func() {
							stream.Wait()
							s.mtx.Unlock()
							e.Client.Self.SetSelfDeafened(true)
						}()
					}
				}
			},

			Disconnect: func(e *gumble.DisconnectEvent) {
				os.Exit(1)
			},
		})
}

type sb struct {
	maxVol  float32
	currVol float32
	iChan   chan Interaction
	mtx     sync.Mutex
}

func OnStartListener(f func()) gumbleutil.Listener {
	var o sync.Once
	return gumbleutil.Listener{
		Connect: func(e *gumble.ConnectEvent) {
			o.Do(f)
		},
	}
}
