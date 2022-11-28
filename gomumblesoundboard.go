package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"layeh.com/gumble/gumble"
	"layeh.com/gumble/gumbleffmpeg"
	"layeh.com/gumble/gumbleutil"
	_ "layeh.com/gumble/opus"
)

//go:embed public
var Assets embed.FS

type File struct {
	Name     string `json:"name"`
	Folder   string `json:"folder"`
	FullPath string
}

func (f File) String() string {
	return f.Folder + "/" + f.Name
}

var soundfiles map[string]File

func scanDirsFunc(l string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}

	validSuffix := []string{
		".mp3",
		".m4a",
		".ogg",
		".flac",
		".opus",
		".wav",
		".MPG",
	}
	validSuffixCheck := false
	for _, s := range validSuffix {
		if strings.HasSuffix(info.Name(), s) {
			validSuffixCheck = true
		}
	}
	if !validSuffixCheck {
		return nil
	}

	if info.IsDir() == false {
		fmt.Printf("File: %s\t%s\n", info.Name(), l)
		dir, file := path.Split(l)
		split := strings.Split(dir, "/")
		f := File{
			FullPath: l,
			Name:     file,
			Folder:   split[len(split)-2],
		}

		soundfiles[f.String()] = f
	}

	return nil
}

func scanDirs(directories []string) {
	soundfiles = make(map[string]File)
	for _, dir := range directories {
		err := filepath.Walk(dir, scanDirsFunc)
		if err != nil {
			fmt.Printf("Error at %s: %v", dir, err)
		}
	}
}

type Interaction struct {
	Stop   bool
	Volume float32
	File   *File
}

var (
	targetChannel   = flag.String("channel", "Root", "channel the bot will join")
	maxVolume       = flag.String("maxvol", "100", "Set the maximum Volume in %, the volume set in the UI is multiplied with it")
	interactionChan = make(chan Interaction)
)

func main() {
	var mtx sync.Mutex
	var maxvol float32

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(FSPrefixer("public", Assets))))
	mux.HandleFunc("/restart", func(_ http.ResponseWriter, _ *http.Request) {
		os.Exit(255)
	})

	mux.HandleFunc("/rescan", func(w http.ResponseWriter, r *http.Request) {
		scanDirs(flag.Args())
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	})

	mux.HandleFunc("/files.json", func(w http.ResponseWriter, r *http.Request) {
		files := make([]File, 0)
		for _, f := range soundfiles {
			files = append(files, File{
				Name:   f.Name,
				Folder: f.Folder,
			})
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(files)
	})

	mux.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		interactionChan <- Interaction{
			Stop: true,
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/volume", func(w http.ResponseWriter, r *http.Request) {
		strVol := r.URL.Query().Get("vol")
		vol, err := strconv.Atoi(strVol)
		if err != nil {
			http.Error(w, fmt.Sprintf("couldn't convert %s to integer: %v", strVol, err), http.StatusBadRequest)
			return
		}

		if vol < 0 && vol > 100 {
			http.Error(w, fmt.Sprintf("number too small or too large: %s", strVol), http.StatusBadRequest)
			return
		}

		volume := float32(vol) / 100 * maxvol
		interactionChan <- Interaction{
			Volume: volume,
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fmt.Sprintf("volume set to %.1f%%", volume*100)))
	})

	mux.HandleFunc("/play", func(w http.ResponseWriter, r *http.Request) {
		unescape := r.URL.Query().Get("file")

		f, ok := soundfiles[unescape]
		if !ok {
			http.Error(w, fmt.Sprintf("%s: file not found", unescape), http.StatusNotFound)
			return
		}

		if !mtx.TryLock() {
			http.Error(w, "already playing a sound, gtfo", http.StatusBadRequest)
			return
		}

		interactionChan <- Interaction{
			File: &f,
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fmt.Sprintf("Playing %s\n", f.FullPath)))
	})

	go func() {
		log.Fatal(http.ListenAndServe(":3000", mux))
	}()

	gumbleutil.Main(
		gumbleutil.AutoBitrate,
		gumbleutil.Listener{
			Connect: func(e *gumble.ConnectEvent) {
				maxVolumeF, err := strconv.Atoi(*maxVolume)
				if err != nil {
					fmt.Printf("Invalid MaxVolume %d", maxVolumeF)
					os.Exit(1)
				}

				maxvol = float32(maxVolumeF) / 100
				fmt.Printf("maximum Volume: %.1f%%\n", maxvol*100)
				fmt.Printf("GoMumbleSoundboard loaded (%d files)\n", len(soundfiles))

				scanDirs(flag.Args())

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

				for interaction := range interactionChan {
					if interaction.Stop == true {
						_ = stream.Stop()
					}

					if interaction.Volume != 0 {
						stream.Volume = interaction.Volume
					}

					if interaction.File != nil {
						e.Client.Self.SetSelfMuted(false)
						stream = gumbleffmpeg.New(e.Client, gumbleffmpeg.SourceFile(interaction.File.FullPath))

						if err := stream.Play(); err != nil {
							return
						}

						go func() {
							stream.Wait()
							mtx.Unlock()
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

type FuncFS func(name string) (fs.File, error)

func (f FuncFS) Open(name string) (fs.File, error) {
	return f(name)
}

func FSPrefixer(prefix string, f fs.FS) fs.FS {
	return FuncFS(func(name string) (fs.File, error) {
		if name == "." {
			name = ""
		}

		if name == "" {
			name = prefix
		} else {
			name = prefix + "/" + name
		}

		return f.Open(name)
	})
}
