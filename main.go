package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/asim/git-http-backend/server"
)

func init() {
	flag.BoolVar(&server.DefaultConfig.RequireAuth, "require_auth", server.DefaultConfig.RequireAuth, "enable basic auth")
	flag.StringVar(&server.DefaultConfig.AuthPassEnvVar, "auth_pass_env_var", server.DefaultConfig.AuthPassEnvVar, "set an env var to provide the basic auth pass as")
	flag.StringVar(&server.DefaultConfig.AuthUserEnvVar, "auth_user_env_var", server.DefaultConfig.AuthUserEnvVar, "set an env var to provide the basic auth user as")
	flag.StringVar(&server.DefaultConfig.DefaultEnv, "default_env", server.DefaultConfig.DefaultEnv, "set the default env")
	flag.StringVar(&server.DefaultConfig.ProjectRoot, "project_root", server.DefaultConfig.ProjectRoot, "set project root")
	flag.StringVar(&server.DefaultConfig.GitBinPath, "git_bin_path", server.DefaultConfig.GitBinPath, "set git bin path")
	flag.StringVar(&server.DefaultAddress, "server_address", server.DefaultAddress, "set server address")
	flag.StringVar(&server.DefaultConfig.RoutePrefix, "route_prefix", server.DefaultConfig.RoutePrefix, "prepend a regex prefix to each git-http-backend route")
}

func main() {
	flag.Parse()

	http.HandleFunc("/", server.Handler())

	if err := http.ListenAndServe(server.DefaultAddress, nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
