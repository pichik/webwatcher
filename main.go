package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/pichik/webwatcher/src/auth"
	"github.com/pichik/webwatcher/src/datacenter"
	"github.com/pichik/webwatcher/src/datacenter/collector"
	"github.com/pichik/webwatcher/src/datacenter/harvester"
	"github.com/pichik/webwatcher/src/misc"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/negroni"
	"golang.org/x/crypto/acme/autocert"
)

func setHeaders(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	if !auth.IsAuthed(r) {
		var headers string
		for key, values := range r.Header {
			for _, value := range values {
				headers += fmt.Sprintf("%s: %s\n", key, value)
			}
		}
		misc.RequestLog.Printf("%-17s %9s %s\n%s", "["+strings.Split(r.RemoteAddr, ":")[0]+"]", "["+r.Method+"]", r.RequestURI, headers)
	}

	origin := r.Header.Get("Origin")

	if origin != "" && origin != "null" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}

	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, PUT, DELETE, PATCH, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Requested-With, Authorization")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.Header().Set("Etag", "W/\"6f2d9-8b3a7c5f0d1e2g3h4i5j6k7l8m9n0p1q\"")
	w.Header().Set("Server", "Google Frontend")
	w.Header().Set("X-Xss-Protection", "mode=block")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "deny")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'none'")

	if r.Method == "OPTIONS" {
		return
	}
	next(w, r)
}

func checkAuth(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	if !auth.IsAuthed(r) {
		setBait(w, r)
		return
	}
	next(w, r)
}
func login(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	next(w, r)
}

func setBait(w http.ResponseWriter, r *http.Request) {
	collector.Bait(w, r)
	w.WriteHeader(http.StatusOK)
}

func main() {
	loadFiles()

	r := mux.NewRouter()
	n := negroni.Classic()

	n.Use(negroni.NewStatic(http.Dir(misc.AssetsDir)))
	n.UseFunc(setHeaders)

	setupAdminRoutes(r)
	setupCollectorRoutes(r)

	r.PathPrefix("/login").Handler(negroni.New(
		negroni.HandlerFunc(auth.Authenticate),
		negroni.HandlerFunc(checkAuth),
	))

	r.HandleFunc(datacenter.DeepCollectorPath, collector.DeepCollect).Methods("POST")
	r.NotFoundHandler = http.HandlerFunc(setBait)

	n.UseHandler(r)

	server, certManager := setupServer(n)
	go http.ListenAndServe(":http", certManager.HTTPHandler(n))
	log.Fatal(server.ListenAndServeTLS("", ""))
}

func setupAdminRoutes(r *mux.Router) {
	adminRoutes := mux.NewRouter().PathPrefix(auth.AdminPanel).Subrouter()
	adminRoutes.HandleFunc("/{id:[a-zA-Z0-9]{64}}", harvester.Extract).Methods("GET")
	adminRoutes.HandleFunc("/{id:[a-zA-Z0-9]{64}}", harvester.Delete).Methods("DELETE")
	adminRoutes.HandleFunc("/all", harvester.ExtractAll).Methods("GET")
	adminRoutes.HandleFunc("/all", harvester.UpdateAll).Methods("POST")
	// adminRoutes.HandleFunc("/all", harvester.DeleteAll).Methods("DELETE")

	r.PathPrefix(auth.AdminPanel).Handler(negroni.New(
		negroni.HandlerFunc(checkAuth),
		negroni.Wrap(adminRoutes),
	))
}

func setupCollectorRoutes(r *mux.Router) {
	collectorRoutes := mux.NewRouter().PathPrefix("/").Subrouter()
	collectorRoutes.HandleFunc(misc.Config.CollectorPath+"{ext:.*\\.[a-zA-Z0-9]+}", collector.GetExtension)
	collectorRoutes.HandleFunc(misc.Config.CollectorPath+"{any:.*}", setBait)

	r.PathPrefix(misc.Config.CollectorPath).Handler(negroni.New(
		negroni.HandlerFunc(collector.SimpleCollect),
		negroni.Wrap(collectorRoutes),
	))
}

func setupServer(n *negroni.Negroni) (*http.Server, autocert.Manager) {
	certManager := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(misc.Config.Hostname, "www."+misc.Config.Hostname),
		Cache:      autocert.DirCache(misc.CertDir),
	}

	server := &http.Server{
		Addr:      ":https",
		Handler:   n,
		TLSConfig: certManager.TLSConfig(),
	}
	return server, certManager
}

func loadFiles() {
	misc.ImportLogs()
	harvester.ImportTemplate()
	collector.ImportTemplate()
	collector.ImportExtensions()
	collector.WebhookLoad()
	datacenter.CreateDatabase()
}
