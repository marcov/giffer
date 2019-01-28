giffer is a simple animated gif file generator written in Go.

## Installation
1. [Install Go](https://golang.org/doc/install#install)
2. Run: `go get github.com/marcov/giffer`

## Usage

> **Note**: if your `$PATH` does not contain `$GOPATH/bin`, you nee to specify it when invoking `giffer`

```
giffer <OPTIONS> DIRECTORY_NAME
```

By default, giffer will generate an animated gif file called `outfile.gif` from
all the jpeg files located at any depth inside `DIRECTORY_NAME`, with a delay
between each gif frame of 100ms.

For more information run `giffer -h`.

