package main

import (
  "fmt"
  "io"
  "io/ioutil"
  "log"
  "net/http"
  "os"
  "os/exec"
  "path"
  "regexp"
  "strings"
  "strconv"
  "time"
)

type Service struct {
  Method string
  Handler func(HandlerReq)
  Rpc string
}

type Config struct {
  ProjectRoot string
  GitBinPath string
  UploadPack bool
  ReceivePack bool
}

type HandlerReq struct {
  w http.ResponseWriter
  r *http.Request
  Rpc string
  Dir string
  File string
}

var config Config = Config{
  ProjectRoot: "/tmp",
  GitBinPath: "/usr/bin/git",
  UploadPack: true,
  ReceivePack: true,
}

var services =  map[string] Service {
  "(.*?)/git-upload-pack$": Service{"POST", service_rpc, "upload-pack"},
  "(.*?)/git-receive-pack$": Service{"POST", service_rpc, "receive-pack"},
  "(.*?)/info/refs$": Service{"GET", get_info_refs, ""},
  "(.*?)/HEAD$": Service{"GET", get_text_file, ""},
  "(.*?)/objects/info/alternates$": Service{"GET", get_text_file, ""},
  "(.*?)/objects/info/http-alternates$": Service{"GET", get_text_file, ""},
  "(.*?)/objects/info/packs$": Service{"GET", get_info_packs, ""},
  "(.*?)/objects/info/[^/]*$": Service{"GET", get_text_file, ""},
  "(.*?)/objects/[0-9a-f]{2}/[0-9a-f]{38}$": Service{"GET", get_loose_object, ""},
  "(.*?)/objects/pack/pack-[0-9a-f]{40}\\.pack$": Service{"GET", get_pack_file, ""},
  "(.*?)/objects/pack/pack-[0-9a-f]{40}\\.idx$": Service{"GET", get_idx_file, ""},
}

// Request handling function

func request_handler() http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    log.Printf("%s %s %s %s", r.RemoteAddr, r.Method, r.URL.Path, r.Proto)
    for match, service := range services {
      re, err := regexp.Compile(match)
      if err != nil {
        log.Print(err)
      }

      if m := re.FindStringSubmatch(r.URL.Path); m != nil {
        if service.Method != r.Method {
          render_method_not_allowed(w, r)
          return
        }

        rpc := service.Rpc
        file := strings.Replace(r.URL.Path, m[1] + "/", "", 1)
        dir, err := get_git_dir(m[1])

        if err != nil {
          log.Print(err)
          render_not_found(w)
          return
        }

        hr := HandlerReq{w, r, rpc, dir, file}
        service.Handler(hr)
        return
      }
    }
    render_not_found(w)
    return
  }
}

// Actual command handling functions

func service_rpc(hr HandlerReq) {
  w, r, rpc, dir := hr.w, hr.r, hr.Rpc, hr.Dir
  access := has_access(r, dir, rpc, true)

  if access == false {
    render_no_access(w)
    return
  }

  input, _ := ioutil.ReadAll(r.Body)

  w.Header().Set("Content-Type", fmt.Sprintf("application/x-git-%s-result", rpc))
  w.WriteHeader(http.StatusOK)

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

  in.Write(input)
  io.Copy(w, stdout)
  cmd.Wait()
}

func get_info_refs(hr HandlerReq) {
  w, r, dir := hr.w, hr.r, hr.Dir
  service_name := get_service_type(r)
  access := has_access(r, dir, service_name, false)

  if access {
    args := []string{service_name, "--stateless-rpc", "--advertise-refs", "."}
    refs := git_command(dir, args...)

    hdr_nocache(w)
    w.Header().Set("Content-Type", fmt.Sprintf("application/x-git-%s-advertisement", service_name))
    w.WriteHeader(http.StatusOK)
    w.Write(packet_write("# service=git-" + service_name + "\n"))
    w.Write(packet_flush())
    w.Write(refs)
  } else {
    update_server_info(dir)
    hdr_nocache(w)
    send_file("text/plain; charset=utf-8", hr)
  }
}

func get_info_packs(hr HandlerReq) {
  hdr_cache_forever(hr.w)
  send_file("text/plain; charset=utf-8", hr)
}

func get_loose_object(hr HandlerReq) {
  hdr_cache_forever(hr.w)
  send_file("application/x-git-loose-object", hr)
}

func get_pack_file(hr HandlerReq) {
  hdr_cache_forever(hr.w)
  send_file("application/x-git-packed-objects", hr)
}

func get_idx_file(hr HandlerReq) {
  hdr_cache_forever(hr.w)
  send_file("application/x-git-packed-objects-toc", hr)
}

func get_text_file(hr HandlerReq) {
  hdr_nocache(hr.w)
  send_file("text/plain", hr)
}

// Logic helping functions

func send_file(content_type string, hr HandlerReq) {
  w, r := hr.w, hr.r
  req_file := path.Join(hr.Dir, hr.File)

  f, err := os.Stat(req_file)
  if os.IsNotExist(err) {
    render_not_found(w)
    return
  }

  w.Header().Set("Content-Type", content_type)
  w.Header().Set("Content-Length", fmt.Sprintf("%d", f.Size()))
  w.Header().Set("Last-Modified", f.ModTime().Format(http.TimeFormat))
  http.ServeFile(w, r, req_file)
}


func get_git_dir(file_path string) (string, error) {
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

func get_service_type(r *http.Request) string {
  service_type := r.FormValue("service")

  if s := strings.HasPrefix(service_type, "git-"); !s {
    return ""
  }

  return strings.Replace(service_type, "git-", "", 1)
}

func has_access(r *http.Request, dir string, rpc string, check_content_type bool) bool {
  if check_content_type {
    if r.Header.Get("Content-Type") != fmt.Sprintf("application/x-git-%s-request", rpc) {
      return false
    }
  }

  if ! (rpc == "upload-pack" || rpc == "receive-pack") {
    return false
  }
  if rpc == "receive-pack" {
    return config.ReceivePack
  }
  if rpc == "upload-pack" {
    return config.UploadPack
  }

  return get_config_setting(rpc, dir)
}

func get_config_setting(service_name string, dir string) bool {
  service_name = strings.Replace(service_name, "-", "", -1)
  setting := get_git_config("http." + service_name, dir)

  if service_name == "uploadpack" {
    return setting != "false"
  }

  return setting == "true"
}

func get_git_config(config_name string, dir string) string {
  args := []string{"config", config_name}
  out := string(git_command(dir, args...))
  return out[0:len(out)-1]
}

func update_server_info(dir string) []byte {
  args := []string{"update-server-info"}
  return git_command(dir, args...)
}

func git_command(dir string, args ...string) []byte {
  command := exec.Command(config.GitBinPath, args...)
  command.Dir = dir
  out, err := command.Output()

  if err != nil {
    log.Print(err)
  }

  return out
}

// HTTP error response handling functions

func render_method_not_allowed(w http.ResponseWriter, r *http.Request) {
  if r.Proto == "HTTP/1.1" {
    w.WriteHeader(http.StatusMethodNotAllowed)
    w.Write([]byte("Method Not Allowed"))
  } else {
    w.WriteHeader(http.StatusBadRequest)
    w.Write([]byte("Bad Request"))
  }
}

func render_not_found(w http.ResponseWriter) {
  w.WriteHeader(http.StatusNotFound)
  w.Write([]byte("Not Found"))
}

func render_no_access(w http.ResponseWriter) {
  w.WriteHeader(http.StatusForbidden)
  w.Write([]byte("Forbidden"))
}

// Packet-line handling function

func packet_flush() []byte {
  return []byte("0000")
}

func packet_write(str string) []byte {
  s := strconv.FormatInt(int64(len(str) + 4), 16)

  if len(s) % 4 != 0 {
    s = strings.Repeat("0", 4 - len(s) % 4) + s
  }

  return []byte(s + str)
}

// Header writing functions

func hdr_nocache(w http.ResponseWriter) {
  w.Header().Set("Expires", "Fri, 01 Jan 1980 00:00:00 GMT")
  w.Header().Set("Pragma", "no-cache")
  w.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
}

func hdr_cache_forever(w http.ResponseWriter) {
  now := time.Now().Unix()
  expires := now + 31536000
  w.Header().Set("Date", fmt.Sprintf("%d", now))
  w.Header().Set("Expires", fmt.Sprintf("%d", expires))
  w.Header().Set("Cache-Control", "public, max-age=31536000")
}

// Main

func main() {
  http.HandleFunc("/", request_handler())

  err := http.ListenAndServe(":8080", nil)
  if err != nil {
    log.Fatal("ListenAndServe: ", err)
  }
}

