package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"time"
)

func getParam(values url.Values,key string,def string) string {
	val := values.Get(key)
	if val == "" {
		return def
	}
	return val
}
func writeSuccess(w http.ResponseWriter,statusCode int,message string ) {
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{ "success": message })
}
func writeError(w http.ResponseWriter,statusCode int,message string ) {
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{ "error": message })
}
func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello, World")
}
func handlerPandoc(w http.ResponseWriter, r *http.Request) {
	if _,err := exec.LookPath("pandoc") ; err != nil {
		writeError(w,400,err.Error())
		return
	}
	body,err := exec.Command("pandoc","-v").Output()
	if err != nil{
		writeError(w,400,err.Error())
		return
	}
	writeSuccess(w,200,string(body))
}

type pandocParams struct {
	url string
	to string
	from string
	stripyaml bool
	jsonObject string
}

type apiResult struct {
	BodyMd string `json:"body_md"`
	BodyHtml string `json:"body_html"`
}

func hoge(w http.ResponseWriter, params *pandocParams){
	client := &http.Client{Timeout: time.Duration(5) * time.Second}
	resp,err := client.Get(params.url)
	if err != nil {
		writeError(w,400,"bad url")
		return
	}
	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()


	cmd := exec.Command("pandoc","-f",params.from,"-t",params.to)
	stdin,err := cmd.StdinPipe()
	if err != nil {
		writeError(w,400,"bad url")
		return
	}
	if params.jsonObject != "" {
		var jsonObj apiResult
		if err := json.Unmarshal(body,&jsonObj) ; err != nil{
			writeError(w,400,err.Error())
			return
		}
		switch params.jsonObject {
		case "body_html":
			body = []byte(jsonObj.BodyHtml)
		case "body_md":
			body = []byte(jsonObj.BodyMd)
		default:
			writeError(w,400,fmt.Sprint(jsonObj))
			return
		}
	}
	if params.stripyaml {
		body = body[bytes.Index(body, []byte("\n---\n"))+5:]
	}
	if _,err := stdin.Write(body) ; err != nil {
		writeError(w,400,err.Error())
		return
	}
	stdin.Close()

	timer := time.AfterFunc(10 * time.Second,func(){
		cmd.Process.Kill()
		writeError(w,400,"pandoc timeout")
		return
	})
	pan, err := cmd.CombinedOutput()

	timer.Stop()
	if err != nil {
		writeError(w,400,err.Error()+"\n"+string(pan))
		return
	}
	w.WriteHeader(200)
	w.Write(pan)
}

func handlerUrl(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm() ; err != nil{
		return
	}
	url := r.Form.Get("url")
	if url == "" {
		writeError(w,400,"url is empty")
		return
	}
	params := &pandocParams{
		url: url,
		to : getParam(r.Form, "to", "html"),
		from : getParam(r.Form, "from", "gfm"),
		stripyaml : (getParam(r.Form, "stripyaml", "true") == "true"),
		jsonObject: "",
	}
	hoge(w,params)
}

func handlerEsa(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm() ; err != nil {
		return
	}
	team := r.Form.Get("team")
	token := r.Form.Get("token")
	num,err := strconv.Atoi(r.Form.Get("num"))
	if team == "" || err != nil || num < 1{
		writeError(w,400,"Bad params")
		return
	}
	print(fmt.Sprintf("https://api.esa.io/v1/teams/%s/posts/%d?access_token=%s",team,num,token))

	jsonObject := "body_" + getParam(r.Form,"type","md")
	fromDefault := "gfm"
	if jsonObject == "body_html" {
		fromDefault = "html"
	}

	params := &pandocParams{
		url: fmt.Sprintf("https://api.esa.io/v1/teams/%s/posts/%d?access_token=%s",team,num,token),
		from : getParam(r.Form, "from", "gfm"),
		to : getParam(r.Form, "to", fromDefault),
		stripyaml: (getParam(r.Form, "stripyaml", "false") == "true"),
		jsonObject: jsonObject,
	}
	hoge(w,params)
}

func main() {
	addr := os.Getenv("ASPANDOC_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	http.HandleFunc("/", handler)
	http.HandleFunc("/version", handlerPandoc )
	http.HandleFunc("/url",handlerUrl)
	http.HandleFunc("/esa",handlerEsa)
	http.HandleFunc("/gist",handlerEsa)
	http.HandleFunc("/snippets",handlerEsa)
	http.ListenAndServe(addr, nil)
}