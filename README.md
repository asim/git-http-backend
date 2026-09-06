# Git HTTP Backend

This is a Go based implementation of Grack (a Rack application), which aimed 
to replace the builtin git-http-backed CGI handler distributed with C Git. 
Grack was written to allow far more webservers to handle Git smart http 
requests. The aim of this project is to improve Git smart http performance by 
utilising the power of Go.

## Dependencies

- Go >= 1.25 to build or embed the server
- Native Git for HTTP push/fetch; repository creation uses go-git in-process

## Install

```
go get github.com/asim/git-http-backend
```

## Usage

Run the backend pointing to a project root and git bin path
```
git-http-backend --project_root=/tmp --git_bin_path=/usr/bin/git
```

Help

```
git-http-backend --help
```

Flags

```
Usage of ./git-http-backend:
  -require_auth bool
        set require auth enable/disable
  -auth_pass_env_var string
        set an env var to provide the basic auth pass as
  -auth_user_env_var string
        set an env var to provide the basic auth user as
  -default_env string
        set the default env
  -git_bin_path string
        set git bin path (default "/usr/bin/git")
  -project_root string
        set project root (default "/tmp")
  -route_prefix string
        prepend a regex prefix to each git-http-backend route
  -server_address string
        set server address (default ":8080")
```

## Server

To embed your own server import and use the package

```go
package main

import (
        "log"
        "net/http"

        "github.com/asim/git-http-backend/server"
)

func main() {
	http.HandleFunc("/", server.Handler())

	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
```

## Repository creation

Use an instance with a storage implementation to create and serve repositories:

```go
store := server.NewFilesystemStore("/srv/git")
srv := server.New(server.DefaultConfig, store)

repo, err := srv.CreateRepository(context.Background(), "team/example.git")
if err != nil {
    log.Fatal(err)
}
log.Printf("Created %s", repo.Path())
log.Fatal(http.ListenAndServe(":8080", srv))
```

`CreateRepository` allocates storage and initializes a bare repository using
go-git, without invoking Git. If initialization fails, it attempts to delete the
allocated storage and reports any cleanup error. `Store.Create` alone only
allocates storage; it does not initialize Git. Filesystem repository names must
end in `.git` and may include namespaces such as `team/example.git`.

Custom stores implement `Open`, `Create`, `Delete`, `Exists`, and `List`.
Repositories currently expose a local filesystem `Path()`, which both go-git
initialization and native Git HTTP operations use. Remote object storage is not
yet supported directly by this interface.

## License

```
(The MIT License)

Copyright (c) 2013 Asim Aslam <asim@aslam.me>

Permission is hereby granted, free of charge, to any person obtaining
a copy of this software and associated documentation files (the
'Software'), to deal in the Software without restriction, including
without limitation the rights to use, copy, modify, merge, publish,
distribute, sublicense, and/or sell copies of the Software, and to
permit persons to whom the Software is furnished to do so, subject to
the following conditions:

The above copyright notice and this permission notice shall be
included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED 'AS IS', WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
```````
