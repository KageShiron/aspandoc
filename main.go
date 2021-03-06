package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

func getParam(values url.Values, key string, def string) string {
	val := values.Get(key)
	if val == "" {
		return def
	}
	return val
}
func writeSuccess(w http.ResponseWriter, statusCode int, message string) {
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{"success": message})
}
func writeError(w http.ResponseWriter, statusCode int, message string) {
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{"error": message})
}
func handler(w http.ResponseWriter, r *http.Request) {
	writeError(w, 404, "Not Found")
}
func handlerPandoc(w http.ResponseWriter, r *http.Request) {
	body, err := exec.Command("docker", "run", "-i", "kageshiron/pandoc", "pandoc","-v").Output()
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeSuccess(w, 200, string(body))
}

type pandocParams struct {
	body      []byte
	to        string
	from      string
	stripyaml bool
}

type apiResult struct {
	BodyMd   string `json:"body_md"`
	BodyHTML string `json:"body_html"`
}

func fetchData(w http.ResponseWriter, url string) []byte {
	client := &http.Client{Timeout: time.Duration(5) * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		writeError(w, 400, "bad url")
		return nil
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		writeError(w, 400, "bad response")
		return nil
	}
	resp.Body.Close()
	return body
}

func pandoc(w http.ResponseWriter, params *pandocParams) {
	cmd := exec.Command("docker", "run", "-i", "kageshiron/pandoc", "pandoc", "-f", params.from, "-t", params.to)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		writeError(w, 500, "Pandoc internal error")
		return
	}
	if params.stripyaml {
		params.body = params.body[bytes.Index(params.body, []byte("\n---\n"))+5:]
	}
	if _, err := stdin.Write(params.body); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	stdin.Close()

	timer := time.AfterFunc(10*time.Second, func() {
		cmd.Process.Kill()
		writeError(w, 400, "pandoc timeout")
		return
	})
	pan, err := cmd.CombinedOutput()

	timer.Stop()
	if err != nil {
		writeError(w, 400, err.Error()+"\n"+string(pan))
		return
	}
	w.WriteHeader(200)
	w.Write(pan)
}

func handlerURL(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		return
	}
	url := r.Form.Get("url")
	if url == "" {
		writeError(w, 400, "url is empty")
		return
	}
	body := fetchData(w, url)
	if body == nil {
		return
	}
	params := &pandocParams{
		body:      body,
		to:        getParam(r.Form, "to", "html"),
		from:      getParam(r.Form, "from", "gfm"),
		stripyaml: (getParam(r.Form, "stripyaml", "false") == "true"),
	}
	pandoc(w, params)
}

func handlerEsa(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		return
	}
	vars := mux.Vars(r)
	num, err := strconv.Atoi(vars["num"])
	if vars["team"] == "" || err != nil || num < 1 {
		writeError(w, 400, "Bad params")
		return
	}

	body := fetchData(w, fmt.Sprintf("https://api.esa.io/v1/teams/%s/posts/%d?access_token=%s", vars["team"], num, vars["token"]))
	if body == nil {
		return
	}

	var jsonObj apiResult
	if err := json.Unmarshal(body, &jsonObj); err != nil {
		writeError(w, 400, err.Error())
		return
	}

	fromDefault := "html"
	if getParam(r.Form, "type", "md") == "html" {
		body = []byte(jsonObj.BodyHTML)
	} else {
		fromDefault = "gfm"
		body = []byte(jsonObj.BodyMd)
	}

	params := &pandocParams{
		body:      body,
		from:      getParam(r.Form, "from", "gfm"),
		to:        getParam(r.Form, "to", fromDefault),
		stripyaml: (getParam(r.Form, "stripyaml", "false") == "true"),
	}
	pandoc(w, params)
}

func handlerGist(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		return
	}
	vars := mux.Vars(r)
	sha := vars["sha"]
	if vars["id"] == "" || strings.Contains(vars["id"], "/") || strings.Contains(sha, "/") {
		writeError(w, 400, "Bad params")
		return
	}
	if sha != "" {
		sha = "/" + sha
	}

	body := fetchData(w, fmt.Sprintf(fmt.Sprintf("https://api.github.com/gists/%s%s", vars["id"], sha)))
	if body == nil {
		return
	}

	var gistResult GistResult
	if err := json.Unmarshal(body, &gistResult); err != nil {
		writeError(w, 400, err.Error())
		return
	}

	content := ""
	if vars["file"] == "" {
		for _, val := range gistResult.Files {
			content += val.Content + "\n"
		}
	}else {
		content = gistResult.Files[vars["file"]].Content
	}	

	params := &pandocParams{
		body:      []byte(content),
		from:      getParam(r.Form, "from", "gfm"),
		to:        getParam(r.Form, "to", "gfm"),
		stripyaml: false,
	}
	pandoc(w, params)
}

func main() {
	addr := os.Getenv("ASPANDOC_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	r := mux.NewRouter().StrictSlash(true)
	r.HandleFunc("/", handler)
	r.HandleFunc("/version", handlerPandoc)
	r.HandleFunc("/url", handlerURL)
	r.HandleFunc("/esa/{team}/{num}", handlerEsa).Queries("token", "{token}")
	r.HandleFunc("/gist/{id}/{file}", handlerGist)
	r.HandleFunc("/gist/{id}", handlerGist)
	// http.HandleFunc("/snippets",handlerEsa)
	srv := &http.Server{
		Handler: r,
		Addr:    addr,
		// Good practice: enforce timeouts for servers you create!
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Fatal(srv.ListenAndServe())
}
