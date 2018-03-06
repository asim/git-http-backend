package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
	"io"

	"github.com/valyala/fasthttp"
)

type Service struct {
	Method  string
	Handler func(HandlerReq)
	Rpc     string
}

type Config struct {
	ProjectRoot string
	GitBinPath  string
	UploadPack  bool
	ReceivePack bool
}

type HandlerReq struct {
	r    *fasthttp.RequestCtx
	Rpc  string
	Dir  string
	File string
}

var config Config = Config{
	ProjectRoot: "/tmp",
	GitBinPath:  "/usr/bin/git",
	UploadPack:  true,
	ReceivePack: true,
}

var services = map[string]Service{
	"(.*?)/git-upload-pack$":                       Service{"POST", serviceRpc, "upload-pack"},
	"(.*?)/git-receive-pack$":                      Service{"POST", serviceRpc, "receive-pack"},
	"(.*?)/info/refs$":                             Service{"GET", getInfoRefs, ""},
	"(.*?)/HEAD$":                                  Service{"GET", getTextFile, ""},
	"(.*?)/objects/info/alternates$":               Service{"GET", getTextFile, ""},
	"(.*?)/objects/info/http-alternates$":          Service{"GET", getTextFile, ""},
	"(.*?)/objects/info/packs$":                    Service{"GET", getInfoPacks, ""},
	"(.*?)/objects/info/[^/]*$":                    Service{"GET", getTextFile, ""},
	"(.*?)/objects/[0-9a-f]{2}/[0-9a-f]{38}$":      Service{"GET", getLooseObject, ""},
	"(.*?)/objects/pack/pack-[0-9a-f]{40}\\.pack$": Service{"GET", getPackFile, ""},
	"(.*?)/objects/pack/pack-[0-9a-f]{40}\\.idx$":  Service{"GET", getIdxFile, ""},
}

var (
	address = ":8080"
)

func init() {
	flag.StringVar(&config.ProjectRoot, "project_root", config.ProjectRoot, "set project root")
	flag.StringVar(&config.GitBinPath, "git_bin_path", config.GitBinPath, "set git bin path")
	flag.StringVar(&address, "server_address", address, "set server address")
}

// Request handling function

func requestHandler(r *fasthttp.RequestCtx) {
	proto := ""
	if r.Request.Header.IsHTTP11() {
		proto = "HTTP 1.1"
	}
	log.Printf("%s %s %s %s", r.RemoteAddr(), r.Method(), r.URI().Path(), proto)
	for match, service := range services {
		re, err := regexp.Compile(match)
		if err != nil {
			log.Print(err)
		}

		if m := re.FindStringSubmatch(string(r.URI().Path())); m != nil {
			if service.Method != string(r.Method()) {
				renderMethodNotAllowed(r)
				return
			}

			rpc := service.Rpc
			file := strings.Replace(string(r.Path()), m[1]+"/", "", 1)
			dir, err := getGitDir(m[1])

			if err != nil {
				log.Print(err)
				renderNotFound(r)
				return
			}

			hr := HandlerReq{r, rpc, dir, file}
			service.Handler(hr)
			return
		}
	}
	renderNotFound(r)
	return

}

// Actual command handling functions

func serviceRpc(hr HandlerReq) {
	r, rpc, dir := hr.r, hr.Rpc, hr.Dir
	access := hasAccess(r, dir, rpc, true)

	if access == false {
		renderNoAccess(r)
		return
	}

	r.SetContentType(fmt.Sprintf("application/x-git-%s-result", rpc))
	r.SetStatusCode(http.StatusOK)

	args := []string{rpc, "--stateless-rpc", dir}
	cmd := exec.Command(config.GitBinPath, args...)
	cmd.Dir = dir
	in, err := cmd.StdinPipe()
	if err != nil {
		log.Print(err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Print(err)
	}

	err = cmd.Start()
	if err != nil {
		log.Print(err)
	}

	switch string(r.Request.Header.Peek("Content-Encoding")) {
	case "gzip":
		body_bytes, _ := r.Request.BodyGunzip()
		in.Write(body_bytes)
	default:
		in.Write(r.PostBody())
	}
	in.Close()
	io.Copy(r, stdout)
	cmd.Wait()
}

func getInfoRefs(hr HandlerReq) {
	r, dir := hr.r, hr.Dir
	service_name := getServiceType(r)
	access := hasAccess(r, dir, service_name, false)

	if access {
		args := []string{service_name, "--stateless-rpc", "--advertise-refs", "."}
		fmt.Println(args)
		refs := gitCommand(dir, args...)

		hdrNocache(r)
		r.Response.Header.Set("Content-Type", fmt.Sprintf("application/x-git-%s-advertisement", service_name))
		r.SetStatusCode(http.StatusOK)
		r.Write(packetWrite("# service=git-" + service_name + "\n"))
		r.Write(packetFlush())
		r.Write(refs)
	} else {
		updateServerInfo(dir)
		hdrNocache(r)
		sendFile("text/plain; charset=utf-8", hr)
	}
}

func getInfoPacks(hr HandlerReq) {
	hdrCacheForever(hr.r)
	sendFile("text/plain; charset=utf-8", hr)
}

func getLooseObject(hr HandlerReq) {
	hdrCacheForever(hr.r)
	sendFile("application/x-git-loose-object", hr)
}

func getPackFile(hr HandlerReq) {
	hdrCacheForever(hr.r)
	sendFile("application/x-git-packed-objects", hr)
}

func getIdxFile(hr HandlerReq) {
	hdrCacheForever(hr.r)
	sendFile("application/x-git-packed-objects-toc", hr)
}

func getTextFile(hr HandlerReq) {
	hdrNocache(hr.r)
	sendFile("text/plain", hr)
}

// Logic helping functions

func sendFile(content_type string, hr HandlerReq) {
	r := hr.r
	req_file := path.Join(hr.Dir, hr.File)

	f, err := os.Stat(req_file)
	if os.IsNotExist(err) {
		renderNotFound(r)
		return
	}
	r.Response.Header.Set("Content-Type", content_type)
	r.Response.Header.Set("Content-Length", fmt.Sprintf("%d", f.Size()))
	r.Response.Header.Set("Last-Modified", f.ModTime().Format(http.TimeFormat))
	fasthttp.ServeFile(r, req_file)
}

func getGitDir(file_path string) (string, error) {
	root := config.ProjectRoot

	if root == "" {
		cwd, err := os.Getwd()

		if err != nil {
			log.Print(err)
			return "", err
		}

		root = cwd
	}

	f := path.Join(root, file_path)
	if _, err := os.Stat(f); os.IsNotExist(err) {
		return "", err
	}

	return f, nil
}

func getServiceType(r *fasthttp.RequestCtx) string {
	service_type := string(r.FormValue("service"))

	if s := strings.HasPrefix(service_type, "git-"); !s {
		return ""
	}

	return strings.Replace(service_type, "git-", "", 1)
}

func hasAccess(r *fasthttp.RequestCtx, dir string, rpc string, check_content_type bool) bool {
	if check_content_type {
		if string(r.Request.Header.ContentType()) != fmt.Sprintf("application/x-git-%s-request", rpc) {
			return false
		}
	}

	if !(rpc == "upload-pack" || rpc == "receive-pack") {
		return false
	}
	if rpc == "receive-pack" {
		return config.ReceivePack
	}
	if rpc == "upload-pack" {
		return config.UploadPack
	}

	return getConfigSetting(rpc, dir)
}

func getConfigSetting(service_name string, dir string) bool {
	service_name = strings.Replace(service_name, "-", "", -1)
	setting := getGitConfig("http."+service_name, dir)

	if service_name == "uploadpack" {
		return setting != "false"
	}

	return setting == "true"
}

func getGitConfig(config_name string, dir string) string {
	args := []string{"config", config_name}
	out := string(gitCommand(dir, args...))
	return out[0 : len(out)-1]
}

func updateServerInfo(dir string) []byte {
	args := []string{"update-server-info"}
	return gitCommand(dir, args...)
}

func gitCommand(dir string, args ...string) []byte {
	command := exec.Command(config.GitBinPath, args...)
	command.Dir = dir
	out, err := command.Output()

	if err != nil {
		log.Print(err)
	}

	return out
}

// HTTP error response handling functions

func renderMethodNotAllowed(r *fasthttp.RequestCtx) {
	if r.Request.Header.IsHTTP11() {
		r.SetStatusCode(http.StatusMethodNotAllowed)
		r.Write([]byte("Method Not Allowed"))
	} else {
		r.SetStatusCode(http.StatusBadRequest)
		r.Write([]byte("Bad Request"))
	}
}

func renderNotFound(r *fasthttp.RequestCtx) {
	r.SetStatusCode(http.StatusNotFound)
	r.Write([]byte("Not Found"))
}

func renderNoAccess(r *fasthttp.RequestCtx) {
	r.SetStatusCode(http.StatusForbidden)
	r.Write([]byte("Forbidden"))
}

// Packet-line handling function

func packetFlush() []byte {
	return []byte("0000")
}

func packetWrite(str string) []byte {
	s := strconv.FormatInt(int64(len(str)+4), 16)

	if len(s)%4 != 0 {
		s = strings.Repeat("0", 4-len(s)%4) + s
	}

	return []byte(s + str)
}

// Header writing functions

func hdrNocache(w *fasthttp.RequestCtx) {
	w.Response.Header.Set("Expires", "Fri, 01 Jan 1980 00:00:00 GMT")
	w.Response.Header.Set("Pragma", "no-cache")
	w.Response.Header.Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
}

func hdrCacheForever(w *fasthttp.RequestCtx) {
	now := time.Now().Unix()
	expires := now + 31536000
	w.Response.Header.Set("Date", fmt.Sprintf("%d", now))
	w.Response.Header.Set("Expires", fmt.Sprintf("%d", expires))
	w.Response.Header.Set("Cache-Control", "public, max-age=31536000")
}

// Main

func main() {
	flag.Parse()

	err := fasthttp.ListenAndServe(address, requestHandler)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
