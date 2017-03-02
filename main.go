package main

import (
	"bytes"
	"flag"
	"crypto/sha256"
	"encoding/hex"
	"github.com/espebra/blobstore"
	"github.com/espebra/blobstore/common"
	"github.com/gorilla/mux"
	"html/template"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

var (
        bindHostFlag = flag.String("host", "127.0.0.1", "Bind host")
        bindPortFlag = flag.Int("port", 8080, "Bind port")
        dataDirFlag = flag.String("directory", "/var/lib/pastebin", "Directory to store pastes")
)

var storage common.Provider

type Paste struct {
	Content  string `json:"content"`
	Checksum string `json:"checksum"`
	Message  string `json:"message"`
	Status   string `json:"status"`
}

func (v Paste) GetName() string {
	hasher := sha256.New()
	hasher.Write([]byte(v.Content))
	return hex.EncodeToString(hasher.Sum(nil))
}

func savePaste(w http.ResponseWriter, r *http.Request) {
	var p Paste

	p.Content = r.FormValue("content")
	p.Checksum = p.GetName()

	if r.FormValue("save") != "" {
		reader := io.Reader(
			bytes.NewReader([]byte(p.Content)),
		)

		nBytes, err := storage.Store(p.Checksum, reader)
		if err != nil {
			log.Println("Unable to write data: %s\n", err)
			p.Message = "Unable to save " + p.Checksum
			p.Status = "error"
		} else {
			p.Message = strconv.FormatInt(nBytes, 10) + " bytes saved as " + p.GetName()
			p.Status = "success"

			http.Redirect(w, r, "/"+p.Checksum, 302)
			return
		}
	}

	data, err := Asset("templates/pastebin.html")
	if err != nil {
		log.Fatalf("Asset not found: %s\n", err)
	}

	t := template.New("paste")
	t, err = t.Parse(string(data))
	if err != nil {
		log.Fatalf("Unable to parse template: %s\n", err)
	}
	err = t.Execute(w, p)
	if err != nil {
		log.Fatalf("Unable to parse template: %s\n", err)
	}
}

func readPaste(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	checksum := vars["checksum"]

	var p Paste

	if checksum != "" {
		var buf bytes.Buffer
		_, err := storage.Retrieve(checksum, &buf)
		if err != nil {
			log.Println(err)
			p.Message = "Paste " + checksum + " does not exist."
			p.Status = "error"
		}
		p.Content = buf.String()
		p.Checksum = p.GetName()
	}

	data, err := Asset("templates/pastebin.html")
	if err != nil {
		log.Fatalf("Asset not found: %s\n", err)
	}

	t := template.New("paste")
	t, err = t.Parse(string(data))
	if err != nil {
		log.Fatalf("Unable to parse template: %s\n", err)
	}
	err = t.Execute(w, p)
	if err != nil {
		log.Fatalf("Unable to parse template: %s\n", err)
	}
}

func main() {
	flag.Parse()
	r := mux.NewRouter()
	r.HandleFunc("/", readPaste).Methods("GET")
	r.HandleFunc("/", savePaste).Methods("POST")
	r.HandleFunc("/{checksum}", readPaste).Methods("GET")
	r.HandleFunc("/{checksum}", savePaste).Methods("POST")
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(assetFS())))

	srv := &http.Server{
		Handler:      r,
		Addr:         *bindHostFlag + ":" + strconv.Itoa(*bindPortFlag),
		WriteTimeout: 10 * time.Second,
		ReadTimeout:  10 * time.Second,
	}

	storage = blobstore.New("filesystem", &common.ProviderData{})
	cfg := map[string]string{}
	cfg["basedir"] = *dataDirFlag
	log.Println("Using basedir " + cfg["basedir"])
	storage.Setup(cfg)

	log.Println("Listening...")
	log.Fatal(srv.ListenAndServe())
}
