// Package server is the reusable server
package server

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Service struct {
	Method  string
	Handler func(*Server, HandlerReq)
	Rpc     string
}

type Config struct {
	RequireAuth    bool
	AuthPassEnvVar string
	AuthUserEnvVar string
	DefaultEnv     string
	ProjectRoot    string
	GitBinPath     string
	UploadPack     bool
	ReceivePack    bool
	RoutePrefix    string
	CommandFunc    func(*exec.Cmd)
}

type HandlerReq struct {
	w    http.ResponseWriter
	r    *http.Request
	Rpc  string
	Dir  string
	File string
}

type Server struct {
	Config Config
	Store  Store
}

var (
	DefaultAddress = ":8080"

	DefaultConfig = Config{
		RequireAuth:    false,
		AuthPassEnvVar: "",
		AuthUserEnvVar: "",
		DefaultEnv:     "",
		ProjectRoot:    "/tmp",
		GitBinPath:     "/usr/bin/git",
		UploadPack:     true,
		ReceivePack:    true,
		RoutePrefix:    "",
		CommandFunc:    func(*exec.Cmd) {},
	}
)

func New(config Config, store Store) *Server {
	if config.GitBinPath == "" {
		config.GitBinPath = "/usr/bin/git"
	}
	if config.CommandFunc == nil {
		config.CommandFunc = func(*exec.Cmd) {}
	}
	if store == nil {
		store = NewFilesystemStore(config.ProjectRoot)
	}
	return &Server{Config: config, Store: store}
}

func NewDefault() *Server {
	return New(DefaultConfig, NewFilesystemStore(DefaultConfig.ProjectRoot))
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

func Handler() http.HandlerFunc {
	return NewDefault().ServeHTTP
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s %s %s", r.RemoteAddr, r.Method, r.URL.Path, r.Proto)
	// Authenticate before opening storage or dispatching any Git HTTP endpoint.
	if s.Config.RequireAuth {
		user, password, ok := r.BasicAuth()
		if !ok || user != s.Config.AuthUserEnvVar || password != s.Config.AuthPassEnvVar {
			renderAuthRequire(w)
			return
		}
	}

	for match, service := range services {
		re, err := regexp.Compile(s.Config.RoutePrefix + match)
		if err != nil {
			log.Print(err)
			continue
		}

		indexes := re.FindStringSubmatchIndex(r.URL.Path)
		if indexes == nil {
			continue
		}

		if service.Method != r.Method {
			renderMethodNotAllowed(w, r)
			return
		}

		matches := re.FindStringSubmatch(r.URL.Path)
		rpc := service.Rpc
		repoName := strings.TrimPrefix(matches[1], "/")
		file := ""
		if len(indexes) >= 4 && indexes[3] >= 0 && indexes[3] <= len(r.URL.Path) {
			file = strings.TrimPrefix(r.URL.Path[indexes[3]:], "/")
		}

		repo, err := s.Store.Open(r.Context(), repoName)
		if err != nil {
			log.Print(err)
			if errors.Is(err, ErrRepositoryNotFound) {
				renderNotFound(w)
			} else {
				renderInternalServerError(w)
			}
			return
		}

		hr := HandlerReq{w: w, r: r, Rpc: rpc, Dir: repo.Path(), File: file}
		service.Handler(s, hr)
		return
	}
	renderNotFound(w)
}

func serviceRpc(s *Server, hr HandlerReq) {
	w, r, rpc, dir := hr.w, hr.r, hr.Rpc, hr.Dir
	access := s.hasAccess(r, dir, rpc, true)

	if access == false {
		renderNoAccess(w)
		return
	}

	w.Header().Set("Content-Type", fmt.Sprintf("application/x-git-%s-result", rpc))
	w.Header().Set("Connection", "Keep-Alive")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	env := os.Environ()
	if s.Config.DefaultEnv != "" {
		env = append(env, s.Config.DefaultEnv)
	}

	user, password, authok := r.BasicAuth()
	if authok {
		if s.Config.AuthUserEnvVar != "" {
			env = append(env, fmt.Sprintf("%s=%s", s.Config.AuthUserEnvVar, user))
		}
		if s.Config.AuthPassEnvVar != "" {
			env = append(env, fmt.Sprintf("%s=%s", s.Config.AuthPassEnvVar, password))
		}
	}

	args := []string{rpc, "--stateless-rpc", dir}
	cmd := exec.CommandContext(r.Context(), s.Config.GitBinPath, args...)
	version := r.Header.Get("Git-Protocol")

	cmd.Dir = dir
	cmd.Env = env
	if len(version) != 0 {
		cmd.Env = append(env, fmt.Sprintf("GIT_PROTOCOL=%s", version))
	}

	s.Config.CommandFunc(cmd)

	in, err := cmd.StdinPipe()
	if err != nil {
		log.Print(err)
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Print(err)
		return
	}
	if err = cmd.Start(); err != nil {
		log.Print(err)
		return
	}

	var reader io.ReadCloser
	switch r.Header.Get("Content-Encoding") {
	case "gzip":
		reader, err = gzip.NewReader(r.Body)
		if err != nil {
			log.Print(err)
			return
		}
		defer reader.Close()
	default:
		reader = r.Body
	}
	_, _ = io.Copy(in, reader)
	_ = in.Close()

	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Print("response writer does not support flushing")
		return
	}

	p := make([]byte, 32*1024)
	for {
		nRead, readErr := stdout.Read(p)
		if nRead > 0 {
			if _, writeErr := w.Write(p[:nRead]); writeErr != nil {
				log.Print(writeErr)
				return
			}
			flusher.Flush()
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			log.Print(readErr)
			break
		}
	}

	if err := cmd.Wait(); err != nil {
		log.Print(err)
	}
}

func getInfoRefs(s *Server, hr HandlerReq) {
	w, r, dir := hr.w, hr.r, hr.Dir
	serviceName := getServiceType(r)
	access := s.hasAccess(r, dir, serviceName, false)
	version := r.Header.Get("Git-Protocol")

	if access {
		args := []string{serviceName, "--stateless-rpc", "--advertise-refs", "."}
		refs := s.gitCommand(r.Context(), dir, version, args...)

		hdrNocache(w)
		w.Header().Set("Content-Type", fmt.Sprintf("application/x-git-%s-advertisement", serviceName))
		w.WriteHeader(http.StatusOK)
		if len(version) == 0 {
			w.Write(packetWrite("# service=git-" + serviceName + "\n"))
			w.Write(packetFlush())
		}
		w.Write(refs)
	} else {
		s.updateServerInfo(r.Context(), dir)
		hdrNocache(w)
		sendFile("text/plain; charset=utf-8", hr)
	}
}

func getInfoPacks(_ *Server, hr HandlerReq) { hdrCacheForever(hr.w); sendFile("text/plain; charset=utf-8", hr) }
func getLooseObject(_ *Server, hr HandlerReq) { hdrCacheForever(hr.w); sendFile("application/x-git-loose-object", hr) }
func getPackFile(_ *Server, hr HandlerReq) { hdrCacheForever(hr.w); sendFile("application/x-git-packed-objects", hr) }
func getIdxFile(_ *Server, hr HandlerReq) { hdrCacheForever(hr.w); sendFile("application/x-git-packed-objects-toc", hr) }
func getTextFile(_ *Server, hr HandlerReq) { hdrNocache(hr.w); sendFile("text/plain", hr) }

func sendFile(contentType string, hr HandlerReq) {
	w, r := hr.w, hr.r
	reqFile := path.Join(hr.Dir, hr.File)

	f, err := os.Stat(reqFile)
	if os.IsNotExist(err) {
		renderNotFound(w)
		return
	}
	if err != nil {
		renderNotFound(w)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", f.Size()))
	w.Header().Set("Last-Modified", f.ModTime().Format(http.TimeFormat))
	http.ServeFile(w, r, reqFile)
}

func getServiceType(r *http.Request) string {
	serviceType := r.FormValue("service")
	if !strings.HasPrefix(serviceType, "git-") {
		return ""
	}
	return strings.Replace(serviceType, "git-", "", 1)
}

func (s *Server) hasAccess(r *http.Request, dir string, rpc string, checkContentType bool) bool {
	if checkContentType && r.Header.Get("Content-Type") != fmt.Sprintf("application/x-git-%s-request", rpc) {
		return false
	}
	if !(rpc == "upload-pack" || rpc == "receive-pack") {
		return false
	}
	if rpc == "receive-pack" {
		return s.Config.ReceivePack
	}
	if rpc == "upload-pack" {
		return s.Config.UploadPack
	}
	return s.getConfigSetting(rpc, dir)
}

func (s *Server) getConfigSetting(serviceName string, dir string) bool {
	serviceName = strings.Replace(serviceName, "-", "", -1)
	setting := s.getGitConfig("http."+serviceName, dir)
	if serviceName == "uploadpack" {
		return setting != "false"
	}
	return setting == "true"
}

func (s *Server) getGitConfig(configName string, dir string) string {
	out := string(s.gitCommand(context.Background(), dir, "", "config", configName))
	return strings.TrimSpace(out)
}

func (s *Server) updateServerInfo(ctx context.Context, dir string) []byte {
	return s.gitCommand(ctx, dir, "", "update-server-info")
}

func (s *Server) gitCommand(ctx context.Context, dir string, version string, args ...string) []byte {
	command := exec.CommandContext(ctx, s.Config.GitBinPath, args...)
	if len(version) > 0 {
		command.Env = append(os.Environ(), fmt.Sprintf("GIT_PROTOCOL=%s", version))
	}
	command.Dir = dir
	s.Config.CommandFunc(command)

	out, err := command.Output()
	if err != nil {
		log.Print(err)
	}
	return out
}

func renderMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	if r.Proto == "HTTP/1.1" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("Method Not Allowed"))
	} else {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Bad Request"))
	}
}

func renderNotFound(w http.ResponseWriter) { w.WriteHeader(http.StatusNotFound); w.Write([]byte("Not Found")) }
func renderInternalServerError(w http.ResponseWriter) { w.WriteHeader(http.StatusInternalServerError); w.Write([]byte("Internal Server Error")) }
func renderNoAccess(w http.ResponseWriter) { w.WriteHeader(http.StatusForbidden); w.Write([]byte("Forbidden")) }
func renderAuthRequire(w http.ResponseWriter) {
	w.Header().Add("Content-Type", "text/plain")
	w.Header().Add("WWW-Authenticate", "Basic realm=\"authorization needed\"")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte("401 Unauthorized"))
}

func packetFlush() []byte { return []byte("0000") }
func packetWrite(str string) []byte {
	s := strconv.FormatInt(int64(len(str)+4), 16)
	if len(s)%4 != 0 {
		s = strings.Repeat("0", 4-len(s)%4) + s
	}
	return []byte(s + str)
}

func hdrNocache(w http.ResponseWriter) {
	w.Header().Set("Expires", "Fri, 01 Jan 1980 00:00:00 GMT")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
}

func hdrCacheForever(w http.ResponseWriter) {
	now := time.Now().Unix()
	expires := now + 31536000
	w.Header().Set("Date", fmt.Sprintf("%d", now))
	w.Header().Set("Expires", fmt.Sprintf("%d", expires))
	w.Header().Set("Cache-Control", "public, max-age=31536000")
}
