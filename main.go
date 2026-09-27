package main

import (
	"log"

	"github.com/LeonardCooray/cloud-fetch/server"
	"github.com/jpillora/opts"
)

var version = "0.0.0-src" //set with ldflags

func main() {
	s := server.Server{
		Title:           "Cloud Fetch",
		Port:            3000,
		HTTPPort:        80,
		ConfigPath:      server.DefaultConfigName,
		SearchConfigURL: server.DefaultSearchConfigURL,
	}

	o := opts.New(&s)
	o.Version(version)
	o.PkgRepo()
	o.SetLineWidth(96)
	o.Parse()

	if err := s.Run(version); err != nil {
		log.Fatal(err)
	}
}
