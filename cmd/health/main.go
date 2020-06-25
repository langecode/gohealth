package main


import (
	"os"
	"path/filepath"

	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	url = kingpin.Flag("url", "url to poll")
)

func main() {
	app := kingpin.New(filepath.Base(os.Args[0]), "Utility to monitor health of http endpoint")
	kingpin.MustParse(app.Parse(os.Args[1:]))
}