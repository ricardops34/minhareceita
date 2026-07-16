package api

import (
	"crypto/subtle"
	"encoding/json/v2"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func (app *api) adminLoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		b, err := adminFiles.ReadFile("login.html")
		if err != nil {
			http.Error(w, "Erro carregando login.", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(b)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Dados inválidos.", 400)
		return
	}
	user, password := os.Getenv("ADMIN_USER"), os.Getenv("ADMIN_PASSWORD")
	validUser := subtle.ConstantTimeCompare([]byte(r.FormValue("username")), []byte(user)) == 1
	validPassword := subtle.ConstantTimeCompare([]byte(r.FormValue("password")), []byte(password)) == 1
	if user == "" || password == "" || !validUser || !validPassword {
		http.Error(w, "Usuário ou senha inválidos.", 401)
		return
	}
	token, err := app.sessions.create()
	if err != nil {
		http.Error(w, "Erro iniciando sessão.", 500)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "minha_receita_admin", Value: token, Path: "/admin", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 43200})
	http.Redirect(w, r, "/admin/", http.StatusSeeOther)
}

func (app *api) adminLogoutHandler(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("minha_receita_admin"); err == nil {
		app.sessions.delete(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "minha_receita_admin", Value: "", Path: "/admin", HttpOnly: true, MaxAge: -1})
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

func writeAdminJSON(w http.ResponseWriter, value any) {
	b, err := json.Marshal(value)
	if err != nil {
		http.Error(w, "Erro processando resposta.", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}

func (app *api) adminJobsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(405)
		return
	}
	writeAdminJSON(w, app.jobs.snapshot())
}

func decodeAdminRequest(r *http.Request, target any) error {
	b, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	if err != nil {
		return err
	}
	return json.Unmarshal(b, target)
}

func (app *api) adminDownloadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	var request struct {
		Month string `json:"month"`
	}
	if decodeAdminRequest(r, &request) != nil {
		app.messageResponse(w, 400, "Competência inválida.")
		return
	}
	request.Month = strings.TrimSpace(request.Month)
	parsed, err := time.Parse("2006-01", request.Month)
	if err != nil || parsed.After(time.Now()) {
		app.messageResponse(w, 400, "Use uma competência válida no formato AAAA-MM.")
		return
	}
	if err := app.jobs.start("download", "download", request.Month, "--directory", "/mnt/data"); err != nil {
		app.messageResponse(w, 409, err.Error())
		return
	}
	writeAdminJSON(w, app.jobs.snapshot())
}

func (app *api) adminTransformHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	var request struct {
		Replace bool `json:"replace"`
	}
	if decodeAdminRequest(r, &request) != nil {
		app.messageResponse(w, 400, "Configuração inválida.")
		return
	}
	args := []string{"transform", "--directory", "/mnt/data", "--skip-graph"}
	if request.Replace {
		args = append(args, "--clean-up")
	}
	if err := app.jobs.start("transform", args...); err != nil {
		app.messageResponse(w, 409, err.Error())
		return
	}
	writeAdminJSON(w, app.jobs.snapshot())
}

func (app *api) adminCancelJobHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	if err := app.jobs.cancel(); err != nil {
		app.messageResponse(w, 409, err.Error())
		return
	}
	writeAdminJSON(w, map[string]bool{"cancelled": true})
}

func (app *api) adminRestartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	if app.jobs.snapshot().Status == "running" {
		app.messageResponse(w, 409, "Aguarde ou cancele o trabalho atual antes de reiniciar.")
		return
	}
	writeAdminJSON(w, map[string]bool{"restarting": true})
	go func() { time.Sleep(300 * time.Millisecond); os.Exit(0) }()
}
