package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const (
	myName                = "gotili"
	defaultHostname       = "localhost"
	listenDefault         = ":9154"
	uApiGet               = "/api/v1/get/"
	uApiNew               = "/api/v1/new"
	uApiCreate            = "/create"
	uGet                  = "/g"
	uInfo                 = "/i"
	uClientShell          = "/gotili-post"
	uFav                  = "/favicon.ico"
	uLogoSmall            = "/gotili-logo-small.png"
	uCss                  = "/custom.css"
	uLogo                 = "/logo.png"
	maxData               = 1048576 // 1MB
	defaultValidity       = 7       // days
	expiryCheck           = 30      // minutes
	defaultMaxClicks      = 1
	crtFile               = myName + ".crt"
	keyFile               = myName + ".key"
	TLSDefault            = false
	notifyDefault         = false
	allowAnonymousDefault = false
)

var (
	auth            TokenDB
	css             []byte
	logo            []byte
	updated         = time.Time{}
	fListen         string
	fURLBase        string
	fTLS            bool
	fNotify         bool
	fAllowAnonymous bool
	scheme          = "http://"
	configDir       = "/etc/" + myName
	userMessageView string
)

type viewInfoEntry struct {
	StoreEntryInfo
	UserMessageView string
}

type jsonError struct {
	Error string `json:"error"`
}

func getRealIP(r *http.Request) string {
	xFF := r.Header.Get("X-Forwarded-For")
	xRI := r.Header.Get("X-Real-IP")
	if xFF != "" {
		return xFF
	} else if xRI != "" {
		return xRI
	}
	return "none"
}

func Log(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		realRemoteAddr := getRealIP(r)
		log.Printf("%s (%s) \"%s %s %s\" \"%s\"", r.RemoteAddr, realRemoteAddr, r.Method, r.URL.Path, r.Proto, r.Header.Get("User-Agent"))
		handler.ServeHTTP(w, r)
	})
}

func updateFiles() {
	auth = makeTokenDB(tryReadFile(authFileName))
	if auth == nil {
		log.Println("auth db could not be loaded, please fix and reload")
	}
	css = tryReadFile(cssFileName)
	logo = tryReadFile(logoFileName)
	userMessageView = fileOrConst(userMessageViewFilename, userMessageViewDefaultText)
	updated = time.Now()
}

func getURLBase() string {
	if fURLBase != "" {
		return fURLBase
	}
	sl := strings.Split(fListen, ":")
	port := sl[len(sl)-1]
	return fmt.Sprintf("%s%s:%s", scheme, defaultHostname, port)
}

func main() {
	flag.StringVar(&fListen, "listen", listenDefault, "listen on IP:port")
	flag.StringVar(&fURLBase, "urlbase", "", "base URL (will be generated by default)")
	flag.BoolVar(&fTLS, "tls", TLSDefault, "use TLS connection")
	flag.BoolVar(&fNotify, "notify", notifyDefault, "send email notification when one time link is used")
	flag.BoolVar(&fAllowAnonymous, "allow-anonymous", allowAnonymousDefault, "allow secrets by anonymous users")
	flag.Parse()

	log.Printf("gotili version %s\n", version)

	store := make(secretStore)
	store.NewEntry("secret", 100, 0, "test@example.org", "test")
	go store.Expiry(time.Minute * expiryCheck)

	updateFiles()

	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)
	go func() {
		for {
			<-sighup
			log.Println("reloading configuration...")
			updateFiles()
		}
	}()

	tIndex := template.New("index")
	tIndex.Parse(htmlMaster)
	tIndex.Parse(htmlIndex)
	tView := template.New("view")
	tView.Parse(htmlMaster)
	tView.Parse(htmlView)
	tViewErr := template.New("viewErr")
	tViewErr.Parse(htmlMaster)
	tViewErr.Parse(htmlViewErr)
	tViewInfo := template.New("viewInfo")
	tViewInfo.Parse(htmlMaster)
	tViewInfo.Parse(htmlViewInfo)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		type Data struct {
			AllowAnonymous bool
		}
		tIndex.ExecuteTemplate(w, "master", &Data{AllowAnonymous: fAllowAnonymous})
	})

	if fAllowAnonymous {
		http.HandleFunc(uApiCreate, func(w http.ResponseWriter, r *http.Request) {
			err := r.ParseForm()
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			entry := store.NewEntry(r.Form.Get("secret"), 1, 7, "anonymous", "")
			w.Write([]byte(fmt.Sprintf("%s%s?id=%s", getURLBase(), uGet, entry)))
		})
	}

	http.HandleFunc(uApiGet, func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Path[len(uApiGet):]
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		if entry, ok := store.GetEntryInfo(id); !ok {
			w.WriteHeader(http.StatusNotFound)
			log.Printf("entry not found: %s", id)
			if jerr := json.NewEncoder(w).Encode(jsonError{"not found"}); jerr != nil {
				panic(jerr)
			}
		} else {
			store.Click(id, r)
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(entry); err != nil {
				panic(err)
			}
		}
	})

	http.HandleFunc(uApiNew, func(w http.ResponseWriter, r *http.Request) {
		var entry StoreEntry
		body, err := io.ReadAll(io.LimitReader(r.Body, maxData))
		if err != nil {
			panic(err)
		}
		if err := r.Body.Close(); err != nil {
			panic(err)
		}
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		if err := json.Unmarshal(body, &entry); err != nil {
			w.WriteHeader(422) // unprocessable entity
			log.Printf("error processing json: %s", err)
			if jerr := json.NewEncoder(w).Encode(jsonError{err.Error()}); jerr != nil {
				panic(jerr)
			}
		} else if !auth.isAuthorized(&entry) {
			w.WriteHeader(http.StatusUnauthorized)
			log.Printf("unauthorized try to make new entry")
			if jerr := json.NewEncoder(w).Encode(jsonError{"unauthorized"}); jerr != nil {
				panic(jerr)
			}
		} else {
			id := store.AddEntry(entry, "")
			newEntry, _ := store.GetEntryInfoHidden(id)
			log.Println("New ID:", id)
			w.WriteHeader(http.StatusCreated)
			if err := json.NewEncoder(w).Encode(newEntry); err != nil {
				panic(err)
			}
		}
	})

	http.HandleFunc(uGet, func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if entry, ok := store.GetEntryInfo(id); !ok {
			w.WriteHeader(http.StatusNotFound)
			log.Printf("entry not found: %s", id)
			tViewErr.ExecuteTemplate(w, "master", nil)
		} else {
			store.Click(id, r)
			w.WriteHeader(http.StatusOK)
			viewEntry := viewInfoEntry{entry, userMessageView}
			tView.ExecuteTemplate(w, "master", viewEntry)
		}
	})

	http.HandleFunc(uInfo, func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if entry, ok := store.GetEntryInfo(id); !ok {
			w.WriteHeader(http.StatusNotFound)
			tViewErr.ExecuteTemplate(w, "master", nil)
		} else {
			w.WriteHeader(http.StatusOK)
			tViewInfo.ExecuteTemplate(w, "master", entry)
		}
	})

	http.HandleFunc(uFav, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/x-icon")
		w.WriteHeader(http.StatusOK)
		w.Write(favicon)
	})

	http.HandleFunc(uLogoSmall, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		w.Write(gotiliLogoSmall)
	})

	http.HandleFunc(uCss, func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, cssFileName, updated, bytes.NewReader(css))
	})

	http.HandleFunc(uLogo, func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, logoFileName, updated, bytes.NewReader(logo))
	})

	http.HandleFunc(uClientShell, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-sh")
		w.WriteHeader(http.StatusOK)
		ClientShellScript(w, getURLBase()+uApiNew)
	})

	if fNotify {
		log.Println("email notifications enabled")
	}

	if fTLS {
		scheme = "https://"
		cf := tryFile(crtFile)
		if cf == "" {
			log.Fatalf("unable to open %s\n", crtFile)
		}
		kf := tryFile(keyFile)
		if kf == "" {
			log.Fatalf("unable to open %s\n", keyFile)
		}
		log.Printf("using '%s' as URL base\n", getURLBase())
		log.Println("listening on", fListen, "with TLS")
		log.Fatal(http.ListenAndServeTLS(fListen, cf, kf, Log(http.DefaultServeMux)))
	} else {
		log.Printf("using '%s' as URL base\n", getURLBase())
		log.Println("listening on", fListen, "without TLS")
		log.Fatal(http.ListenAndServe(fListen, Log(http.DefaultServeMux)))
	}
}
