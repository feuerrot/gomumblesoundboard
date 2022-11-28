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
	"strconv"
)

//go:embed public
var Assets embed.FS

func (s *sb) setupHTTP() {
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
		s.iChan <- Interaction{
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

		volume := float32(vol) / 100 * s.maxVol
		s.iChan <- Interaction{
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

		if !s.mtx.TryLock() {
			http.Error(w, "already playing a sound, gtfo", http.StatusBadRequest)
			return
		}

		s.iChan <- Interaction{
			File: &f,
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fmt.Sprintf("Playing %s\n", f.FullPath)))
	})

	go func() {
		log.Fatal(http.ListenAndServe(":3000", mux))
	}()
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
