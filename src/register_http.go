package main

import (
	"html/template"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/iagoMAO/Botzin.OpenMASE/authentication"
	"github.com/iagoMAO/Botzin.OpenMASE/utils"
	"github.com/rs/zerolog/log"
)

// Accounts a single IP may create per window
const (
	registerLimit  = 5
	registerWindow = time.Hour
)

var registerPage = template.Must(template.New("register").Parse(`<!doctype html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Botzin!!! - Cadastro</title>
<style>
	body { font-family: sans-serif; background: #111; color: #eee; display: flex; justify-content: center; padding: 48px 16px; }
	form { background: #1d1d1d; padding: 24px; border-radius: 8px; width: 100%; max-width: 320px; }
	h1 { color: #E3FF0E; margin-top: 0; }
	label { display: block; margin-top: 12px; }
	input { width: 100%; box-sizing: border-box; padding: 8px; margin-top: 4px; }
	button { margin-top: 16px; width: 100%; padding: 10px; background: #E3FF0E; border: 0; font-weight: bold; cursor: pointer; }
	.error { color: #FF3333; }
	.ok { color: #00FF00; }
</style>
</head>
<body>
<form method="post" action="/register">
	<h1>BOTZIN!!!</h1>
	{{if .Error}}<p class="error">{{.Error}}</p>{{end}}
	{{if .Success}}<p class="ok">Conta <b>{{.Nick}}</b> criada! Já pode entrar no jogo.</p>{{end}}
	<label>Nick <input name="nick" maxlength="16" required value="{{.Nick}}"></label>
	<label>Senha <input name="password" type="password" maxlength="32" required></label>
	<label>Confirmar senha <input name="confirm" type="password" maxlength="32" required></label>
	<button type="submit">Criar conta</button>
</form>
</body>
</html>`))

type registerView struct {
	Nick    string
	Error   string
	Success bool
}

type rateLimiter struct {
	mutex sync.Mutex
	hits  map[string][]time.Time
}

func (r *rateLimiter) allow(key string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	now := time.Now()

	var recent []time.Time
	for _, hit := range r.hits[key] {
		if now.Sub(hit) < registerWindow {
			recent = append(recent, hit)
		}
	}

	if len(recent) >= registerLimit {
		r.hits[key] = recent
		return false
	}

	r.hits[key] = append(recent, now)
	return true
}

func newRegisterHandler() http.Handler {
	limiter := &rateLimiter{hits: make(map[string][]time.Time)}
	mux := http.NewServeMux()

	render := func(w http.ResponseWriter, status int, view registerView) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		registerPage.Execute(w, view)
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		render(w, http.StatusOK, registerView{})
	})

	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		nick := r.FormValue("nick")
		password := r.FormValue("password")

		if password != r.FormValue("confirm") {
			render(w, http.StatusBadRequest, registerView{Nick: nick, Error: "As senhas não conferem."})
			return
		}

		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}

		if !limiter.allow(host) {
			render(w, http.StatusTooManyRequests, registerView{Nick: nick, Error: "Muitos cadastros, tente mais tarde."})
			return
		}

		id, err := authentication.Register(nick, password, cfg.STARTING_CREDITS, cfg.STARTING_GOLD)
		if err != nil {
			render(w, http.StatusBadRequest, registerView{Nick: nick, Error: err.Error()})
			return
		}

		log.Info().Msgf("REGISTER - %s created account %s (id %d)", host, nick, id)
		render(w, http.StatusOK, registerView{Nick: nick, Success: true})
	})

	return mux
}

func StartRegister() {
	defer utils.Recover("REGISTER listener")

	server := &http.Server{
		Addr:              ":" + cfg.REGISTER_PORT,
		Handler:           newRegisterHandler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		log.Error().Msgf("Listening error: %s", err)
		return
	}

	log.Info().Msgf("REGISTER - Successfully started listening on port %s.", cfg.REGISTER_PORT)

	if err := server.Serve(listener); err != nil {
		log.Error().Msgf("Register server error: %s", err)
	}
}
