# Git HTTP Backend

This is a Go based implementation of Grack (a Rack application), which aimed 
to replace the builtin git-http-backed CGI handler distributed with C Git. 
Grack was written to allow far more webservers to handle Git smart http 
requests. The aim of this project is to improve Git smart http performance by 
utilising the power of Go.

## Dependencies

- Git >= 1.7

## Install

```
go get github.com/asim/git-http-backend
```

## Usage

```
git-http-backend --project_root=/tmp --git_bin_path=/usr/bin/git
```

## Server

To embed your own server import and use the package

```
import "github.com/asim/git-http-backend/server"
```

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
```
